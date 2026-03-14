package conversation

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"smart-speaker/internal/graph"
	oauthgooglecalendar "smart-speaker/internal/oauth/googlecalendar"
	"smart-speaker/internal/state"
	types "smart-speaker/internal/types"
)

var (
	markdownLinkPattern = regexp.MustCompile(`\[[^\]]*\]\((https?://[^)]+)\)`)
	bareURLPattern      = regexp.MustCompile(`https?://\S+`)
	citationPattern     = regexp.MustCompile("cite[^]+")
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
	Status          UtteranceStatus
	ResponseID      string
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

	timer  *time.Timer
	timerC <-chan time.Time

	pendingTimeline    []aiSegment
	pendingTimelineIdx int

	pendingRequestID        string
	pendingRequestCancelled bool
	invalidResponseRetries  int

	seq int

	logFile    *os.File
	logWriter  *bufio.Writer
	logEncoder *json.Encoder

	calendarContextCache string
}

const (
	maxInvalidResponseRetries = 1
	invalidResponseRetryHint  = "前回の出力はJSONとして無効でした。必ずJSONのみを返してください。出力は {\"timeline\":[{\"type\":\"wait\",\"sec\":整数},{\"type\":\"speech\",\"text\":\"文字列\"}]} の形式に従ってください。"
	diaryPromptPrefix         = "以下は過去の会話をまとめた日記です。参考として扱ってください。\n"
	calendarPromptPrefix      = "以下はGoogleカレンダー情報です。会話の参考にしてください。\n\n"
	calendarCreateToolName    = "google_calendar_create"
	calendarUpdateToolName    = "google_calendar_update"
	calendarPromptDays        = 3
	calendarFetchMaxResults   = 30
)

type logRecord struct {
	Timestamp  string `json:"ts"`
	Speaker    string `json:"speaker"`
	Text       string `json:"text"`
	ResponseID string `json:"response_id,omitempty"`
	Source     string `json:"source,omitempty"`
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
			return
		case evt, ok := <-r.upstream:
			if !ok {
				r.stopTimer()
				return
			}
			r.handleEvent(evt)
		case <-r.timerC:
			r.timerC = nil
			r.timer = nil
			r.advanceTimeline()
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
	case types.EventTimerFired:
		timerEvt, ok := evt.Payload.(types.TimerFiredEvent)
		if !ok {
			return
		}
		r.handleTimerFired(timerEvt)
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
	r.clearPendingTimeline()
	r.cancelPendingRequest()
	r.invalidResponseRetries = 0
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

func (r *runner) handleTimerFired(evt types.TimerFiredEvent) {
	text := strings.TrimSpace(evt.ReminderText)
	if text == "" {
		return
	}
	r.emit(types.Event{
		Kind: types.EventRealtimeOutput,
		Payload: types.OutputLine{
			Role:   "assistant",
			Text:   text,
			Source: "timer",
		},
	})
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
		if r.retryInvalidResponse() {
			return
		}
		return
	}
	r.invalidResponseRetries = 0
	root := r.buildUtteranceChain(out)
	if len(root) == 0 {
		return
	}
	r.pendingTimeline = root
	r.pendingTimelineIdx = 0
	r.advanceTimeline()
}

func (r *runner) handleToolResponse(resp types.ToolResponse) {
	name := strings.TrimSpace(resp.Name)
	if name == "" {
		name = "unknown_tool"
	}
	if name == calendarCreateToolName || name == calendarUpdateToolName {
		r.calendarContextCache = ""
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
	r.clearPendingTimeline()
	r.cancelPendingRequest()
	r.cancelUnplayedUtterances()
	if r.current != nil && r.current.Status == UtterancePlaying {
		r.current.Status = UtteranceCanceled
		r.emit(types.Event{Kind: types.EventTTSCancel, Payload: types.TTSCancel{ResponseID: r.current.ResponseID}})
	}
	r.current = nil
	r.utteranceByResponseID = make(map[string]*Utterance)
	r.conversation = nil
	r.calendarContextCache = ""
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
	if !r.hasPendingSpeech() {
		r.clearPendingTimeline()
		r.updateConversationState()
		return
	}
	waitSec := r.consumeLeadingWaitSeconds()
	if !r.hasPendingSpeech() {
		r.clearPendingTimeline()
		r.updateConversationState()
		return
	}
	r.startTimer(r.estimateWaitDuration(tts, waitSec))
	r.updateConversationState()
}

func (r *runner) requestResponse(messages []types.ChatMessage) {
	messages = r.withSystemContexts(messages)
	if len(messages) == 0 {
		return
	}
	r.invalidResponseRetries = 0
	reqID := r.nextID("req")
	r.pendingRequestID = reqID
	r.pendingRequestCancelled = false
	r.emit(types.Event{Kind: types.EventResponsesRequest, Payload: types.ResponsesRequest{
		RequestID: reqID,
		Messages:  messages,
	}})
}

func withDiaryContext(messages []types.ChatMessage) []types.ChatMessage {
	diary := strings.TrimSpace(state.GetDiaryContent())
	if diary == "" {
		return messages
	}
	withDiary := make([]types.ChatMessage, 0, len(messages)+1)
	withDiary = append(withDiary, types.ChatMessage{
		Role:    "system",
		Content: diaryPromptPrefix + diary,
	})
	withDiary = append(withDiary, messages...)
	return withDiary
}

func (r *runner) withSystemContexts(messages []types.ChatMessage) []types.ChatMessage {
	out := r.withCalendarContext(messages)
	return withDiaryContext(out)
}

func (r *runner) withCalendarContext(messages []types.ChatMessage) []types.ChatMessage {
	content := strings.TrimSpace(r.calendarContextCache)
	if content == "" {
		built, err := r.buildCalendarContext()
		if err != nil {
			log.Printf("conversation: failed to build calendar context: %v", err)
			return messages
		}
		content = strings.TrimSpace(built)
		r.calendarContextCache = content
	}
	if content == "" {
		return messages
	}
	withCalendar := make([]types.ChatMessage, 0, len(messages)+1)
	withCalendar = append(withCalendar, types.ChatMessage{
		Role:    "system",
		Content: content,
	})
	withCalendar = append(withCalendar, messages...)
	return withCalendar
}

func (r *runner) buildCalendarContext() (string, error) {
	if _, err := oauthgooglecalendar.LoadToken(); err != nil {
		return "", nil
	}
	if r.ctx == nil {
		return "", nil
	}
	ctx, cancel := context.WithTimeout(r.ctx, 8*time.Second)
	defer cancel()
	token, err := oauthgooglecalendar.AccessToken(ctx)
	if err != nil {
		return "", err
	}
	now := time.Now()
	day0 := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	dayN := day0.AddDate(0, 0, calendarPromptDays)
	events, err := fetchPrimaryCalendarEvents(ctx, token, day0, dayN, calendarFetchMaxResults)
	if err != nil {
		return "", err
	}
	return formatCalendarPrompt(events, day0), nil
}

func (r *runner) retryInvalidResponse() bool {
	if r.invalidResponseRetries >= maxInvalidResponseRetries {
		log.Printf("conversation: invalid response retry exhausted (%d/%d)", r.invalidResponseRetries, maxInvalidResponseRetries)
		return false
	}
	messages := r.buildConversationMessages()
	messages = r.withSystemContexts(messages)
	if len(messages) == 0 {
		return false
	}
	messages = append(messages, types.ChatMessage{
		Role:    "system",
		Content: invalidResponseRetryHint,
	})
	r.invalidResponseRetries++
	reqID := r.nextID("req")
	r.pendingRequestID = reqID
	r.pendingRequestCancelled = false
	r.emit(types.Event{Kind: types.EventResponsesRequest, Payload: types.ResponsesRequest{
		RequestID: reqID,
		Messages:  messages,
		Tools:     []any{},
	}})
	log.Printf("conversation: retrying due to invalid response (%d/%d)", r.invalidResponseRetries, maxInvalidResponseRetries)
	return true
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
		r.advanceTimeline()
		return
	}

	utt.ResponseID = r.nextID("resp")
	r.current = utt
	r.utteranceByResponseID[utt.ResponseID] = utt

	r.logRecord(r.buildLogRecord(utt))
	state.SetLastActivityAt(time.Now())
	line := types.OutputLine{
		Role:       "assistant",
		Text:       utt.Content,
		ResponseID: utt.ResponseID,
		Source:     utteranceSource(utt),
	}
	r.emit(types.Event{Kind: types.EventRealtimeOutput, Payload: line})
	r.emit(types.Event{Kind: types.EventRealtimeOutput, Payload: types.OutputLine{
		Role:       "assistant",
		ResponseID: utt.ResponseID,
		Final:      true,
		Source:     utteranceSource(utt),
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

func (r *runner) clearPendingTimeline() {
	r.pendingTimeline = nil
	r.pendingTimelineIdx = 0
}

func (r *runner) hasPendingSpeech() bool {
	for i := r.pendingTimelineIdx; i < len(r.pendingTimeline); i++ {
		if r.pendingTimeline[i].Type == "speech" {
			return true
		}
	}
	return false
}

func (r *runner) consumeLeadingWaitSeconds() float64 {
	var total float64
	for r.pendingTimelineIdx < len(r.pendingTimeline) {
		seg := r.pendingTimeline[r.pendingTimelineIdx]
		if seg.Type != "wait" {
			break
		}
		total += normalizeWaitSeconds(seg.Sec)
		r.pendingTimelineIdx++
	}
	return total
}

func (r *runner) advanceTimeline() {
	for r.pendingTimelineIdx < len(r.pendingTimeline) {
		seg := r.pendingTimeline[r.pendingTimelineIdx]
		r.pendingTimelineIdx++
		switch seg.Type {
		case "wait":
			delay := waitDelay(seg.Sec)
			if delay > 0 {
				r.startTimer(delay)
				return
			}
		case "speech":
			speech := sanitizeSpeech(seg.Text)
			if speech == "" {
				continue
			}
			utt := &Utterance{
				ID:      r.nextID("ai"),
				Speaker: SpeakerAI,
				Content: speech,
				Status:  UtteranceUnplayed,
			}
			r.appendUtterance(utt)
			r.playUtterance(utt)
			return
		}
	}
	r.clearPendingTimeline()
}

func (r *runner) startTimer(d time.Duration) {
	if d <= 0 {
		r.stopTimer()
		r.advanceTimeline()
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
	Timeline []aiSegment `json:"timeline"`
}

type calendarEventsResponse struct {
	Items []calendarEvent `json:"items"`
}

type calendarEvent struct {
	Summary string                `json:"summary"`
	Start   calendarEventDateTime `json:"start"`
	End     calendarEventDateTime `json:"end"`
}

type calendarEventDateTime struct {
	Date     string `json:"date"`
	DateTime string `json:"dateTime"`
}

type aiSegment struct {
	Type string `json:"type"`
	Sec  *int   `json:"sec,omitempty"`
	Text string `json:"text,omitempty"`
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
	if len(out.Timeline) == 0 {
		return aiOutput{}, false
	}
	speechCount := 0
	for _, seg := range out.Timeline {
		switch seg.Type {
		case "wait":
			if seg.Sec == nil {
				return aiOutput{}, false
			}
		case "speech":
			if strings.TrimSpace(seg.Text) == "" {
				return aiOutput{}, false
			}
			speechCount++
		default:
			return aiOutput{}, false
		}
	}
	if speechCount == 0 {
		return aiOutput{}, false
	}
	return out, true
}

func (r *runner) buildUtteranceChain(out aiOutput) []aiSegment {
	if len(out.Timeline) == 0 {
		return nil
	}
	timeline := make([]aiSegment, 0, len(out.Timeline))
	speechCount := 0
	for _, seg := range out.Timeline {
		switch seg.Type {
		case "wait":
			wait := sanitizeWait(seg.Sec)
			timeline = append(timeline, aiSegment{Type: "wait", Sec: &wait})
		case "speech":
			text := sanitizeSpeech(seg.Text)
			if text == "" {
				continue
			}
			timeline = append(timeline, aiSegment{Type: "speech", Text: text})
			speechCount++
		}
	}
	if speechCount == 0 {
		return nil
	}
	return timeline
}

func postWaitDelay(value float64) time.Duration {
	if value <= 0 {
		return 0
	}
	return time.Duration(value * float64(time.Second))
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

func sanitizeWait(value *int) int {
	if value == nil {
		return 0
	}
	if *value < 0 {
		return 0
	}
	if *value > 5 {
		return 5
	}
	return *value
}

func waitDelay(value *int) time.Duration {
	return postWaitDelay(normalizeWaitSeconds(value))
}

func normalizeWaitSeconds(value *int) float64 {
	if value == nil {
		return 0
	}
	sec := sanitizeWait(value)
	return float64(sec)
}

func utteranceSource(_ *Utterance) string {
	return "conversation"
}

func fetchPrimaryCalendarEvents(ctx context.Context, token string, start time.Time, end time.Time, maxResults int) ([]calendarEvent, error) {
	if maxResults <= 0 {
		maxResults = calendarFetchMaxResults
	}
	query := url.Values{}
	query.Set("timeMin", start.Format(time.RFC3339))
	query.Set("timeMax", end.Format(time.RFC3339))
	query.Set("singleEvents", "true")
	query.Set("orderBy", "startTime")
	query.Set("maxResults", strconv.Itoa(maxResults))
	endpoint := "https://www.googleapis.com/calendar/v3/calendars/primary/events?" + query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 300 {
			msg = msg[:300] + "..."
		}
		return nil, fmt.Errorf("google calendar events list failed: status=%d body=%s", resp.StatusCode, msg)
	}
	var parsed calendarEventsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	return parsed.Items, nil
}

func formatCalendarPrompt(events []calendarEvent, day0 time.Time) string {
	labels := []string{"今日", "明日", "明後日"}
	grouped := make([][]string, calendarPromptDays)
	dayIndex := make(map[string]int, calendarPromptDays)
	for i := 0; i < calendarPromptDays; i++ {
		d := day0.AddDate(0, 0, i)
		dayIndex[d.Format("2006-01-02")] = i
	}
	for _, event := range events {
		startAt, ok := eventStartTime(event.Start)
		if !ok {
			continue
		}
		idx, ok := dayIndex[startAt.Format("2006-01-02")]
		if !ok {
			continue
		}
		grouped[idx] = append(grouped[idx], formatCalendarEventLine(event))
	}
	var b strings.Builder
	b.WriteString(calendarPromptPrefix)
	for i := 0; i < calendarPromptDays; i++ {
		b.WriteString("[")
		b.WriteString(labels[i])
		b.WriteString("]\n")
		lines := grouped[i]
		if len(lines) == 0 {
			b.WriteString("- 予定なし\n")
		} else {
			for _, line := range lines {
				b.WriteString("- ")
				b.WriteString(line)
				b.WriteString("\n")
			}
		}
		if i < calendarPromptDays-1 {
			b.WriteString("\n")
		}
	}
	return strings.TrimSpace(b.String())
}

func formatCalendarEventLine(event calendarEvent) string {
	title := strings.TrimSpace(event.Summary)
	if title == "" {
		title = "(タイトルなし)"
	}
	start := formatCalendarEventClock(event.Start, false)
	end := formatCalendarEventClock(event.End, true)
	if start == "" && end == "" {
		return title
	}
	if end == "" {
		return strings.TrimSpace(start + " " + title)
	}
	return strings.TrimSpace(start + "-" + end + " " + title)
}

func eventStartTime(start calendarEventDateTime) (time.Time, bool) {
	if start.DateTime != "" {
		t, err := time.Parse(time.RFC3339, start.DateTime)
		if err != nil {
			return time.Time{}, false
		}
		local := t.In(time.Local)
		return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.Local), true
	}
	if start.Date != "" {
		t, err := time.ParseInLocation("2006-01-02", start.Date, time.Local)
		if err != nil {
			return time.Time{}, false
		}
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local), true
	}
	return time.Time{}, false
}

func formatCalendarEventClock(dt calendarEventDateTime, isEnd bool) string {
	if dt.Date != "" && dt.DateTime == "" {
		if isEnd {
			return ""
		}
		return "終日"
	}
	if dt.DateTime != "" {
		if t, err := time.Parse(time.RFC3339, dt.DateTime); err == nil {
			return t.In(time.Local).Format("15:04")
		}
	}
	return ""
}

func (r *runner) estimateWaitDuration(tts types.TTSEvent, waitSec float64) time.Duration {
	waitDuration := postWaitDelay(waitSec)
	startAt := tts.AudioStartAt
	if startAt.IsZero() {
		return waitDuration
	}
	if tts.DurationSeconds <= 0 {
		return waitDuration
	}
	endAt := startAt.Add(time.Duration(tts.DurationSeconds * float64(time.Second)))
	remaining := time.Until(endAt)
	if remaining < 0 {
		remaining = 0
	}
	return remaining + waitDuration
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
	return logRecord{
		Speaker:    "ai",
		Text:       utt.Content,
		ResponseID: utt.ResponseID,
		Source:     utteranceSource(utt),
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
