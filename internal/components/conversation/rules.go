package conversation

import (
	"regexp"
	"strings"
	"time"

	types "smart-speaker/internal/types"
)

var (
	markdownLinkPattern = regexp.MustCompile(`\[[^\]]*\]\((https?://[^)]+)\)`)
	bareURLPattern      = regexp.MustCompile(`https?://\S+`)
	citationPattern     = regexp.MustCompile("cite[^]+")
)

// ここから下は、会話イベントを処理するルールを実行順に列挙する領域です。

// 会話イベントに適用するルールを実行順に返す。
func defaultConversationRules() []Rule {
	return []Rule{
		speechStartRule{},
		humanTextRule{},
		timerFiredRule{},
		responsesRule{},
		responsesStreamRule{},
		toolResponseRule{},
		sessionClearRule{},
		ttsEndRule{},
		timerElapsedRule{},
	}
}

type speechStartRule struct{}

// 発話開始時に進行中の会話処理を中断する。
func (speechStartRule) Apply(core *conversationCore, sig signal) ([]effect, bool) {
	if _, ok := sig.(speechStartSignal); !ok {
		return nil, false
	}
	return core.interruptCurrentConversationEffects(), true
}

type humanTextRule struct{}

// 人間の入力を履歴に追加し、AI応答リクエストを開始する。
func (humanTextRule) Apply(core *conversationCore, sig signal) ([]effect, bool) {
	s, ok := sig.(humanTextSignal)
	if !ok {
		return nil, false
	}
	effects := core.interruptCurrentConversationEffects()
	core.state.appendUtterance(&Utterance{
		ID:      core.state.nextID("human"),
		Speaker: SpeakerHuman,
		StartAt: time.Now(),
		Content: s.text,
		Status:  UtterancePlayed,
	})
	effects = append(effects, logRecordEffect{
		record: logRecord{
			Speaker: "human",
			Text:    s.text,
		},
	})
	messages := core.state.buildConversationMessages()
	effects = append(effects,
		emitConversationActivityEffect(time.Now(), "human_turn_committed"),
		emitConversationSnapshotEffect(messages),
	)
	effects = append(effects, core.buildResponseRequestEffect(messages, nil)...)
	return effects, true
}

type timerFiredRule struct{}

// タイマー通知の読み上げテキストを出力イベントとして流す。
func (timerFiredRule) Apply(_ *conversationCore, sig signal) ([]effect, bool) {
	s, ok := sig.(timerFiredSignal)
	if !ok {
		return nil, false
	}
	text := strings.TrimSpace(s.event.ReminderText)
	if text == "" {
		return nil, true
	}
	return []effect{emitEventEffect{event: types.Event{
		Kind: types.EventRealtimeOutput,
		Payload: types.OutputLine{
			Role:   "assistant",
			Text:   text,
			Source: "timer",
		},
	}}}, true
}

type responsesRule struct{}

// AI応答を検証し、ホワイトボード更新や読み上げタイムラインへ変換する。
func (responsesRule) Apply(core *conversationCore, sig signal) ([]effect, bool) {
	s, ok := sig.(responsesSignal)
	if !ok {
		return nil, false
	}
	resp := s.response
	if resp.RequestID == "" || resp.RequestID != core.state.pendingRequestID {
		return nil, true
	}
	if core.state.pendingRequestCancelled {
		core.state.pendingRequestCancelled = false
		core.state.pendingRequestID = ""
		return nil, true
	}
	if len(resp.ToolCalls) > 0 || !resp.HasResponse {
		return nil, true
	}

	core.state.pendingRequestID = ""
	out, parsed := parseAIOutput(resp.Text)
	if !parsed {
		effects := []effect{runtimeLogEffect{
			message: "conversation: invalid response: " + strings.TrimSpace(resp.Text),
		}}
		effects = append(effects, core.retryInvalidResponseEffects()...)
		return effects, true
	}

	core.state.invalidResponseRetries = 0
	var effects []effect
	if out.Whiteboard != nil {
		effects = append(effects, emitEventEffect{
			event: types.Event{
				Kind: types.EventWhiteboardUpdate,
				Payload: types.WhiteboardUpdate{
					Content: out.Whiteboard.Content,
				},
			},
		})
	}
	root := buildTimelineSegments(out)
	if len(root) == 0 {
		return effects, true
	}
	core.state.pendingTimeline = root
	core.state.pendingTimelineIdx = 0
	effects = append(effects, core.advanceTimelineEffects()...)
	return effects, true
}

// AI応答のtimelineを、実行可能な待機・発話セグメントへ変換する。
func buildTimelineSegments(out aiOutput) []timelineSegment {
	if len(out.Timeline) == 0 {
		return nil
	}
	timeline := make([]timelineSegment, 0, len(out.Timeline))
	speechCount := 0
	for _, seg := range out.Timeline {
		switch seg.Type {
		case "wait":
			timeline = append(timeline, timelineSegment{
				Type:    "wait",
				WaitSec: sanitizeWait(seg.Sec),
			})
		case "speech":
			text := sanitizeSpeech(seg.Text)
			if text == "" {
				continue
			}
			timeline = append(timeline, timelineSegment{Type: "speech", Text: text})
			speechCount++
		}
	}
	if speechCount == 0 {
		return nil
	}
	return timeline
}

// 読み上げ本文からURLや引用表記を取り除く。
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

// 待機秒数を許容範囲に丸める。
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

type responsesStreamRule struct{}

// AI応答ストリームの各チャンクを処理し、逐次タイムラインへ反映する。
func (responsesStreamRule) Apply(core *conversationCore, sig signal) ([]effect, bool) {
	s, ok := sig.(responsesStreamChunkSignal)
	if !ok {
		return nil, false
	}
	chunk := s.chunk
	if chunk.RequestID == "" || chunk.RequestID != core.state.pendingRequestID {
		return nil, true
	}
	if core.state.pendingRequestCancelled {
		core.state.pendingRequestCancelled = false
		core.state.pendingRequestID = ""
		core.state.pendingRequestStreaming = false
		return nil, true
	}
	core.state.pendingRequestStreaming = true
	if strings.TrimSpace(chunk.Err) != "" {
		return core.failStream(chunk.Err, false), true
	}
	if chunk.Done {
		return core.completeStream(), true
	}
	if core.state.pendingStreamFailed {
		return nil, true
	}
	line := strings.TrimSpace(chunk.Line)
	parsed, ok := parseAIChunk(line)
	if !ok {
		return core.failStream("conversation: invalid stream chunk: "+line, true), true
	}

	switch parsed.Type {
	case "speech":
		text := sanitizeSpeech(parsed.Text)
		if text == "" {
			return core.failStream("conversation: invalid stream speech after sanitize: "+line, true), true
		}
		core.state.pendingStreamSpeechStarted = true
		core.state.pendingTimeline = append(core.state.pendingTimeline, timelineSegment{Type: "speech", Text: text})
		if core.state.current == nil && !core.state.pendingTimelineTimerWaiting {
			return core.advanceTimelineEffects(), true
		}
		return nil, true
	case "wait":
		core.state.pendingTimeline = append(core.state.pendingTimeline, timelineSegment{
			Type:    "wait",
			WaitSec: sanitizeWait(parsed.Sec),
		})
		if core.state.current == nil && !core.state.pendingTimelineTimerWaiting {
			return core.advanceTimelineEffects(), true
		}
		return nil, true
	case "whiteboard":
		return []effect{emitEventEffect{
			event: types.Event{
				Kind: types.EventWhiteboardUpdate,
				Payload: types.WhiteboardUpdate{
					Content: parsed.Content,
				},
			},
		}}, true
	default:
		return core.failStream("conversation: invalid stream chunk: "+line, true), true
	}
}

// ストリーム失敗時に状態を片付け、必要なら再試行effectを作る。
func (c *conversationCore) failStream(message string, retryInvalid bool) []effect {
	effects := []effect{runtimeLogEffect{message: message}}
	c.state.pendingStreamFailed = true
	c.state.pendingRequestStreaming = false
	if c.state.pendingStreamSpeechStarted {
		c.state.pendingRequestID = ""
		c.state.clearPendingTimeline()
		return effects
	}
	c.state.pendingRequestID = ""
	c.state.clearPendingTimeline()
	if retryInvalid {
		effects = append(effects, c.retryInvalidResponseEffects()...)
	}
	return effects
}

// ストリーム完了時に状態を確定し、必要なら会話スナップショットを通知する。
func (c *conversationCore) completeStream() []effect {
	c.state.pendingRequestStreaming = false
	c.state.pendingRequestID = ""
	if !c.state.pendingStreamSpeechStarted {
		c.state.clearPendingTimeline()
		effects := []effect{runtimeLogEffect{
			message: "conversation: invalid stream response: no speech chunk",
		}}
		effects = append(effects, c.retryInvalidResponseEffects()...)
		return effects
	}
	c.state.invalidResponseRetries = 0
	if c.state.current != nil || c.state.hasPendingSpeech() || c.state.pendingTimelineTimerWaiting {
		return nil
	}
	c.state.clearPendingTimeline()
	return []effect{emitConversationSnapshotEffect(c.state.buildConversationMessages())}
}

type toolResponseRule struct{}

// ツール実行結果を会話履歴とログへ反映する。
func (toolResponseRule) Apply(core *conversationCore, sig signal) ([]effect, bool) {
	s, ok := sig.(toolResponseSignal)
	if !ok {
		return nil, false
	}
	resp := s.response
	name := strings.TrimSpace(resp.Name)
	if name == "" {
		name = "unknown_tool"
	}
	var effects []effect
	if name == "write_diary" {
		return effects, true
	}
	output := strings.TrimSpace(string(resp.Output))
	if output == "" {
		return effects, true
	}
	content := "ツール実行結果(" + name + "): " + output
	core.state.appendUtterance(&Utterance{
		ID:         core.state.nextID("tool"),
		Speaker:    SpeakerTool,
		StartAt:    time.Now(),
		Content:    content,
		Status:     UtterancePlayed,
		ResponseID: strings.TrimSpace(resp.ResponseID),
	})
	effects = append(effects, logRecordEffect{
		record: logRecord{
			Speaker:    "tool",
			Text:       content,
			ResponseID: strings.TrimSpace(resp.ResponseID),
			Source:     name,
		},
	})
	effects = append(effects, emitConversationSnapshotEffect(core.state.buildConversationMessages()))
	return effects, true
}

type sessionClearRule struct{}

// セッション消去時に会話状態をリセットして空のスナップショットを通知する。
func (sessionClearRule) Apply(core *conversationCore, sig signal) ([]effect, bool) {
	if _, ok := sig.(sessionClearSignal); !ok {
		return nil, false
	}
	effects := core.interruptCurrentConversationEffects()
	core.state.resetConversation()
	effects = append(effects, emitConversationSnapshotEffect(nil))
	return effects, true
}

type ttsEndRule struct{}

// TTS終了時に発話状態を更新し、次の待機または発話へ進める。
func (ttsEndRule) Apply(core *conversationCore, sig signal) ([]effect, bool) {
	s, ok := sig.(ttsEndSignal)
	if !ok {
		return nil, false
	}
	respID := strings.TrimSpace(s.event.ResponseID)
	if respID == "" {
		return nil, true
	}
	utt := core.state.utteranceByResponseID[respID]
	if utt == nil {
		return nil, true
	}
	if utt.Status == UtterancePlaying {
		utt.Status = UtterancePlayed
		utt.DurationSeconds = s.event.DurationSeconds
	}
	delete(core.state.utteranceByResponseID, respID)
	if core.state.current == utt {
		core.state.current = nil
	}
	effects := []effect{}
	if !core.state.hasPendingSpeech() {
		if core.state.pendingRequestStreaming {
			effects = append(effects, emitConversationSnapshotEffect(core.state.buildConversationMessages()))
			return effects, true
		}
		core.state.clearPendingTimeline()
		effects = append(effects, emitConversationSnapshotEffect(core.state.buildConversationMessages()))
		return effects, true
	}
	waitSec := core.state.consumeLeadingWaitSeconds()
	if !core.state.hasPendingSpeech() {
		core.state.clearPendingTimeline()
		effects = append(effects, emitConversationSnapshotEffect(core.state.buildConversationMessages()))
		return effects, true
	}
	effects = append(effects,
		startTimerEffect{duration: estimateWaitDuration(s.event, waitSec)},
		emitConversationSnapshotEffect(core.state.buildConversationMessages()),
	)
	core.state.pendingTimelineTimerWaiting = true
	return effects, true
}

// TTS再生終了時刻を考慮して、次の待機時間を見積もる。
func estimateWaitDuration(tts types.TTSEvent, waitSec float64) time.Duration {
	waitDuration := time.Duration(waitSec * float64(time.Second))
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

type timerElapsedRule struct{}

// 待機タイマー終了時にタイムラインの次の発話へ進める。
func (timerElapsedRule) Apply(core *conversationCore, sig signal) ([]effect, bool) {
	if _, ok := sig.(timerElapsedSignal); !ok {
		return nil, false
	}
	core.state.pendingTimelineTimerWaiting = false
	return core.advanceTimelineEffects(), true
}

// 会話イベントを処理するルールの列挙はここまでです。
