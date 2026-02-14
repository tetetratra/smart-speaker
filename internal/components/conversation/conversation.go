package conversation

import (
	"bufio"
	"context"
	"encoding/json"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"smart-speaker/internal/graph"
	"smart-speaker/internal/state"
	types "smart-speaker/internal/types"
)

var (
	markdownLinkPattern = regexp.MustCompile(`\[[^\]]*\]\((https?://[^)]+)\)`)
	bareURLPattern      = regexp.MustCompile(`https?://\S+`)
	citationPattern     = regexp.MustCompile(`\uE200cite\uE202[^\uE201]+\uE201`)
)

type Config struct {
	LogPath string
}

type Speaker string

const (
	SpeakerHuman Speaker = "human"
	SpeakerAI    Speaker = "ai"
	SpeakerTool  Speaker = "tool"
)

type UtteranceStatus int

const (
	UtteranceUnplayed UtteranceStatus = iota
	UtterancePlaying
	UtterancePlayed
	UtteranceCanceled
)

type Utterance struct {
	ID              string
	Speaker         Speaker
	StartAt         time.Time
	DurationSeconds float64
	Content         string
	Chain           *Utterance
	PostWaitSec     float64
	PrePauseSec     float64
	Status          UtteranceStatus
	ResponseID      string
	IsChain         bool
}

type runner struct {
	upstream   chan types.Event
	downstream chan types.Event
	ctx        context.Context
	cancel     context.CancelFunc
	once       bool

	conversation          []*Utterance
	current               *Utterance
	utteranceByResponseID map[string]*Utterance

	timer         *time.Timer
	timerC        <-chan time.Time
	preplayTimer  *time.Timer
	preplayTimerC <-chan time.Time

	pendingChain   *Utterance
	pendingPreplay *Utterance

	pendingRequestID        string
	pendingRequestCancelled bool

	seq int

	logFile    *os.File
	logWriter  *bufio.Writer
	logEncoder *json.Encoder
}

type logRecord struct {
	Timestamp  string   `json:"ts"`
	Speaker    string   `json:"speaker"`
	Text       string   `json:"text"`
	ResponseID string   `json:"response_id,omitempty"`
	Source     string   `json:"source,omitempty"`
	PrePause   *float64 `json:"pre_pause,omitempty"`
	PostWait   *float64 `json:"post_wait,omitempty"`
	IsChain    bool     `json:"is_chain,omitempty"`
}

// NewStage は会話タイミング管理のステージを作成します。
func NewStage(cfg Config) *graph.Stage {
	logWriter, logEncoder, logFile := openLogWriter(cfg.LogPath)
	r := &runner{
		upstream:              make(chan types.Event, graph.DefaultChannelBufferSize),
		downstream:            make(chan types.Event, graph.DefaultChannelBufferSize),
		utteranceByResponseID: make(map[string]*Utterance),
		logWriter:             logWriter,
		logEncoder:            logEncoder,
		logFile:               logFile,
	}
	return &graph.Stage{
		Upstream:   r.upstream,
		Downstream: r.downstream,
		Run:        r.run,
		CloseFn:    r.close,
	}
}

func (r *runner) run(parent context.Context) {
	r.ctx, r.cancel = context.WithCancel(parent)
	go r.consume()
}

func (r *runner) consume() {
	defer close(r.downstream)
	for {
		select {
		case <-r.ctx.Done():
			r.stopTimer()
			r.stopPreplayTimer()
			return
		case evt, ok := <-r.upstream:
			if !ok {
				r.stopTimer()
				r.stopPreplayTimer()
				return
			}
			r.handleEvent(evt)
		case <-r.timerC:
			r.timerC = nil
			r.timer = nil
			r.handleNoHumanResponse()
		case <-r.preplayTimerC:
			r.preplayTimerC = nil
			r.preplayTimer = nil
			r.handlePreplayReady()
		}
	}
}

func (r *runner) handleEvent(evt types.Event) {
	switch evt.Kind {
	case types.EventSpeechStart:
		r.handleSpeechStart()
	case types.EventTextInput:
		line, ok := evt.Payload.(types.OutputLine)
		if !ok {
			return
		}
		text := strings.TrimSpace(line.Text)
		if text == "" {
			return
		}
		r.handleHumanText(text)
	case types.EventResponsesResponse:
		resp, ok := evt.Payload.(types.ResponsesResponse)
		if !ok {
			return
		}
		r.handleResponses(resp)
	case types.EventToolResponse:
		resp, ok := evt.Payload.(types.ToolResponse)
		if !ok {
			return
		}
		r.handleToolResponse(resp)
	case types.EventSessionClear:
		r.handleSessionClear()
	case types.EventTTSEnd:
		tts, ok := evt.Payload.(types.TTSEvent)
		if !ok {
			return
		}
		r.handleTTSEnd(tts)
	}
}

func (r *runner) handleSpeechStart() {
	r.stopTimer()
	r.stopPreplayTimer()
	r.pendingChain = nil
	r.pendingPreplay = nil
	r.cancelPendingRequest()
	r.cancelUnplayedUtterances()
	if r.current != nil && r.current.Status == UtterancePlaying {
		r.current.Status = UtteranceCanceled
		r.emit(types.Event{Kind: types.EventTTSCancel, Payload: types.TTSCancel{ResponseID: r.current.ResponseID}})
		delete(r.utteranceByResponseID, r.current.ResponseID)
		r.current = nil
	}
}

func (r *runner) handleHumanText(text string) {
	r.handleSpeechStart()

	r.appendUtterance(&Utterance{
		ID:      r.nextID("human"),
		Speaker: SpeakerHuman,
		StartAt: time.Now(),
		Content: text,
		Status:  UtterancePlayed,
	})
	r.logRecord(logRecord{
		Speaker: "human",
		Text:    text,
	})
	r.updateConversationState()
	r.requestResponse(r.buildConversationMessages())
}

func (r *runner) handleResponses(resp types.ResponsesResponse) {
	if resp.RequestID == "" || resp.RequestID != r.pendingRequestID {
		return
	}
	if r.pendingRequestCancelled {
		r.pendingRequestCancelled = false
		r.pendingRequestID = ""
		return
	}
	if len(resp.ToolCalls) > 0 {
		return
	}
	if !resp.HasResponse {
		return
	}
	r.pendingRequestID = ""
	out, ok := parseAIOutput(resp.Text)
	if !ok {
		log.Printf("conversation: invalid response: %s", strings.TrimSpace(resp.Text))
		return
	}
	root := r.buildUtteranceChain(out)
	if root == nil {
		return
	}
	r.appendUtterance(root)
	for next := root.Chain; next != nil; next = next.Chain {
		r.appendUtterance(next)
	}
	delay := prePauseDelay(root.PrePauseSec)
	if delay <= 0 {
		r.playUtterance(root)
		return
	}
	r.pendingPreplay = root
	r.startPreplayTimer(delay)
}

func (r *runner) handleToolResponse(resp types.ToolResponse) {
	name := strings.TrimSpace(resp.Name)
	if name == "" {
		name = "unknown_tool"
	}
	if name == "write_diary" {
		return
	}
	output := strings.TrimSpace(string(resp.Output))
	if output == "" {
		return
	}
	content := "ツール実行結果(" + name + "): " + output
	r.appendUtterance(&Utterance{
		ID:         r.nextID("tool"),
		Speaker:    SpeakerTool,
		StartAt:    time.Now(),
		Content:    content,
		Status:     UtterancePlayed,
		ResponseID: strings.TrimSpace(resp.ResponseID),
	})
	r.logRecord(logRecord{
		Speaker:    "tool",
		Text:       content,
		ResponseID: strings.TrimSpace(resp.ResponseID),
		Source:     name,
	})
	r.updateConversationState()
}

func (r *runner) handleSessionClear() {
	r.stopTimer()
	r.stopPreplayTimer()
	r.pendingChain = nil
	r.pendingPreplay = nil
	r.cancelPendingRequest()
	r.cancelUnplayedUtterances()
	if r.current != nil && r.current.Status == UtterancePlaying {
		r.current.Status = UtteranceCanceled
		r.emit(types.Event{Kind: types.EventTTSCancel, Payload: types.TTSCancel{ResponseID: r.current.ResponseID}})
	}
	r.current = nil
	r.utteranceByResponseID = make(map[string]*Utterance)
	r.conversation = nil
	state.ClearConversationMessages()
}

func (r *runner) handleTTSEnd(tts types.TTSEvent) {
	respID := strings.TrimSpace(tts.ResponseID)
	if respID == "" {
		return
	}
	utt := r.utteranceByResponseID[respID]
	if utt == nil {
		return
	}
	if utt.Status == UtterancePlaying {
		utt.Status = UtterancePlayed
		utt.DurationSeconds = tts.DurationSeconds
	}
	delete(r.utteranceByResponseID, respID)
	if r.current == utt {
		r.current = nil
	}
	state.SetLastActivityAt(time.Now())

	r.pendingChain = utt.Chain
	r.startTimer(r.estimateWaitDuration(utt, tts))
	r.updateConversationState()
}

func (r *runner) handleNoHumanResponse() {
	if r.pendingChain != nil {
		next := r.pendingChain
		r.pendingChain = nil
		r.playUtterance(next)
	}
}

func (r *runner) handlePreplayReady() {
	next := r.pendingPreplay
	r.pendingPreplay = nil
	if next == nil || next.Status == UtteranceCanceled {
		return
	}
	r.playUtterance(next)
}

func (r *runner) requestResponse(messages []types.ChatMessage) {
	if len(messages) == 0 {
		return
	}
	reqID := r.nextID("req")
	r.pendingRequestID = reqID
	r.pendingRequestCancelled = false
	r.emit(types.Event{Kind: types.EventResponsesRequest, Payload: types.ResponsesRequest{
		RequestID: reqID,
		Messages:  messages,
	}})
}

func (r *runner) buildConversationMessages() []types.ChatMessage {
	var out []types.ChatMessage
	for _, utt := range r.conversation {
		if utt == nil {
			continue
		}
		switch utt.Speaker {
		case SpeakerHuman:
			out = append(out, types.ChatMessage{Role: "user", Content: utt.Content})
		case SpeakerAI:
			if utt.Status != UtterancePlayed {
				continue
			}
			out = append(out, types.ChatMessage{Role: "assistant", Content: utt.Content})
		case SpeakerTool:
			out = append(out, types.ChatMessage{Role: "system", Content: utt.Content})
		}
	}
	return out
}

func (r *runner) playUtterance(utt *Utterance) {
	if utt == nil {
		return
	}
	utt.Status = UtterancePlaying
	utt.StartAt = time.Now()
	if strings.TrimSpace(utt.Content) == "" {
		r.logRecord(r.buildLogRecord(utt))
		utt.Status = UtterancePlayed
		utt.DurationSeconds = 0
		state.SetLastActivityAt(time.Now())
		r.pendingChain = utt.Chain
		r.startTimer(postWaitDelay(utt.PostWaitSec))
		return
	}

	utt.ResponseID = r.nextID("resp")
	r.current = utt
	r.utteranceByResponseID[utt.ResponseID] = utt

	exp := clampExpectation(utt.PostWaitSec)
	prePause := utt.PrePauseSec
	postWait := utt.PostWaitSec
	r.logRecord(r.buildLogRecord(utt))
	state.SetLastActivityAt(time.Now())
	line := types.OutputLine{
		Role:        "assistant",
		Text:        utt.Content,
		ResponseID:  utt.ResponseID,
		Source:      utteranceSource(utt),
		Expectation: &exp,
		PrePauseSec: &prePause,
		PostWaitSec: &postWait,
	}
	r.emit(types.Event{Kind: types.EventRealtimeOutput, Payload: line})
	r.emit(types.Event{Kind: types.EventRealtimeOutput, Payload: types.OutputLine{
		Role:        "assistant",
		ResponseID:  utt.ResponseID,
		Final:       true,
		Source:      utteranceSource(utt),
		Expectation: &exp,
		PrePauseSec: &prePause,
		PostWaitSec: &postWait,
	}})
}

func (r *runner) updateConversationState() {
	state.SetConversationMessages(r.buildConversationMessages())
}

func (r *runner) appendUtterance(utt *Utterance) {
	if utt == nil {
		return
	}
	r.conversation = append(r.conversation, utt)
}

func (r *runner) cancelPendingRequest() {
	if r.pendingRequestID == "" {
		return
	}
	r.pendingRequestCancelled = true
}

func (r *runner) cancelUnplayedUtterances() {
	for _, utt := range r.conversation {
		if utt == nil || utt.Speaker != SpeakerAI {
			continue
		}
		if utt.Status == UtterancePlayed {
			continue
		}
		utt.Status = UtteranceCanceled
	}
}

func (r *runner) startTimer(d time.Duration) {
	if d <= 0 {
		r.stopTimer()
		r.handleNoHumanResponse()
		return
	}
	if r.timer == nil {
		r.timer = time.NewTimer(d)
		r.timerC = r.timer.C
		return
	}
	if !r.timer.Stop() {
		select {
		case <-r.timer.C:
		default:
		}
	}
	r.timer.Reset(d)
	r.timerC = r.timer.C
}

func (r *runner) stopTimer() {
	if r.timer == nil {
		return
	}
	if !r.timer.Stop() {
		select {
		case <-r.timer.C:
		default:
		}
	}
	r.timer = nil
	r.timerC = nil
}

func (r *runner) startPreplayTimer(d time.Duration) {
	if d <= 0 {
		r.stopPreplayTimer()
		r.handlePreplayReady()
		return
	}
	if r.preplayTimer == nil {
		r.preplayTimer = time.NewTimer(d)
		r.preplayTimerC = r.preplayTimer.C
		return
	}
	if !r.preplayTimer.Stop() {
		select {
		case <-r.preplayTimer.C:
		default:
		}
	}
	r.preplayTimer.Reset(d)
	r.preplayTimerC = r.preplayTimer.C
}

func (r *runner) stopPreplayTimer() {
	if r.preplayTimer == nil {
		return
	}
	if !r.preplayTimer.Stop() {
		select {
		case <-r.preplayTimer.C:
		default:
		}
	}
	r.preplayTimer = nil
	r.preplayTimerC = nil
}

func (r *runner) emit(evt types.Event) {
	select {
	case <-r.ctx.Done():
		return
	case r.downstream <- evt:
	}
}

func (r *runner) close() error {
	if r.once {
		return nil
	}
	r.once = true
	if r.cancel != nil {
		r.cancel()
	}
	if r.logWriter != nil {
		if err := r.logWriter.Flush(); err != nil {
			log.Printf("conversation: log flush error: %v", err)
		}
	}
	if r.logFile != nil {
		if err := r.logFile.Close(); err != nil {
			log.Printf("conversation: log close error: %v", err)
		}
	}
	close(r.upstream)
	return nil
}

func (r *runner) nextID(prefix string) string {
	r.seq++
	return prefix + "_" + strconv.Itoa(r.seq)
}

type aiOutput struct {
	PrePause float64     `json:"pre_pause"`
	Messages []aiMessage `json:"messages"`
}

type aiMessage struct {
	Speech   string  `json:"speech"`
	PostWait float64 `json:"post_wait"`
}

func parseAIOutput(raw string) (aiOutput, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return aiOutput{}, false
	}
	var out aiOutput
	dec := json.NewDecoder(strings.NewReader(trimmed))
	if err := dec.Decode(&out); err != nil {
		return aiOutput{}, false
	}
	return out, true
}

func (r *runner) buildUtteranceChain(out aiOutput) *Utterance {
	if len(out.Messages) == 0 {
		return nil
	}
	rootSpeech := sanitizeSpeech(out.Messages[0].Speech)
	root := &Utterance{
		ID:          r.nextID("ai"),
		Speaker:     SpeakerAI,
		Content:     rootSpeech,
		PostWaitSec: clampPostWait(out.Messages[0].PostWait),
		PrePauseSec: clampPrePause(out.PrePause),
		Status:      UtteranceUnplayed,
	}
	cur := root
	for _, entry := range out.Messages[1:] {
		speech := sanitizeSpeech(entry.Speech)
		next := &Utterance{
			ID:          r.nextID("ai"),
			Speaker:     SpeakerAI,
			Content:     speech,
			PostWaitSec: clampPostWait(entry.PostWait),
			Status:      UtteranceUnplayed,
			IsChain:     true,
		}
		cur.Chain = next
		cur = next
	}
	return root
}

func postWaitDelay(value float64) time.Duration {
	if value <= 0 {
		return 0
	}
	return time.Duration(value * float64(time.Second))
}

func prePauseDelay(value float64) time.Duration {
	if value <= 0 {
		return 0
	}
	return time.Duration(value * float64(time.Second))
}

func clampPostWait(value float64) float64 {
	if value <= 0 {
		return 0.5
	}
	if value > 5 {
		return 5
	}
	return value
}

func clampPrePause(value float64) float64 {
	if value <= 0 {
		return 0.5
	}
	if value > 5 {
		return 5
	}
	return value
}

func clampExpectation(value float64) int {
	v := int(math.Ceil(value))
	if v < 1 {
		return 1
	}
	if v > 5 {
		return 5
	}
	return v
}

func sanitizeSpeech(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	out := markdownLinkPattern.ReplaceAllString(trimmed, "")
	out = bareURLPattern.ReplaceAllString(out, "")
	out = citationPattern.ReplaceAllString(out, "")
	return strings.TrimSpace(out)
}

func utteranceSource(utt *Utterance) string {
	if utt == nil {
		return "conversation"
	}
	if utt.IsChain {
		return "conversation-chain"
	}
	return "conversation"
}

func (r *runner) estimateWaitDuration(utt *Utterance, tts types.TTSEvent) time.Duration {
	if utt == nil {
		return 0
	}
	expectation := postWaitDelay(utt.PostWaitSec)
	startAt := tts.AudioStartAt
	if startAt.IsZero() {
		return expectation
	}
	if tts.DurationSeconds <= 0 {
		return expectation
	}
	endAt := startAt.Add(time.Duration(tts.DurationSeconds * float64(time.Second)))
	remaining := time.Until(endAt)
	if remaining < 0 {
		remaining = 0
	}
	return remaining + expectation
}

func openLogWriter(path string) (*bufio.Writer, *json.Encoder, *os.File) {
	logPath := strings.TrimSpace(path)
	if logPath == "" {
		return nil, nil, nil
	}
	dir := filepath.Dir(logPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("conversation: failed to create log dir: %v", err)
		return nil, nil, nil
	}
	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("conversation: failed to open log file: %v", err)
		return nil, nil, nil
	}
	writer := bufio.NewWriter(file)
	encoder := json.NewEncoder(writer)
	return writer, encoder, file
}

func (r *runner) buildLogRecord(utt *Utterance) logRecord {
	var prePause *float64
	if utt.PrePauseSec > 0 {
		value := utt.PrePauseSec
		prePause = &value
	}
	var postWait *float64
	if utt.PostWaitSec > 0 {
		value := utt.PostWaitSec
		postWait = &value
	}
	return logRecord{
		Speaker:    "ai",
		Text:       utt.Content,
		ResponseID: utt.ResponseID,
		Source:     utteranceSource(utt),
		PrePause:   prePause,
		PostWait:   postWait,
		IsChain:    utt.IsChain,
	}
}

func (r *runner) logRecord(rec logRecord) {
	if r.logEncoder == nil {
		return
	}
	if rec.Timestamp == "" {
		rec.Timestamp = time.Now().Format(time.RFC3339Nano)
	}
	if err := r.logEncoder.Encode(rec); err != nil {
		log.Printf("conversation: log encode error: %v", err)
		return
	}
	if err := r.logWriter.Flush(); err != nil {
		log.Printf("conversation: log flush error: %v", err)
	}
}
