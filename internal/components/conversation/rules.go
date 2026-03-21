package conversation

import (
	"log"
	"strings"
	"time"

	types "smart-speaker/internal/types"
)

func defaultConversationRules() []Rule {
	return []Rule{
		speechStartRule{},
		humanTextRule{},
		timerFiredRule{},
		responsesRule{},
		toolResponseRule{},
		sessionClearRule{},
		ttsEndRule{},
		timerElapsedRule{},
	}
}

type speechStartRule struct{}

func (speechStartRule) Apply(core *conversationCore, sig signal) ([]effect, bool) {
	if _, ok := sig.(speechStartSignal); !ok {
		return nil, false
	}
	return core.interruptCurrentConversationEffects(), true
}

type humanTextRule struct{}

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
	root := buildUtteranceChain(out)
	if len(root) == 0 {
		return effects, true
	}
	core.state.pendingTimeline = root
	core.state.pendingTimelineIdx = 0
	effects = append(effects, core.advanceTimelineEffects()...)
	return effects, true
}

type toolResponseRule struct{}

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
	return effects, true
}

type timerElapsedRule struct{}

func (timerElapsedRule) Apply(core *conversationCore, sig signal) ([]effect, bool) {
	if _, ok := sig.(timerElapsedSignal); !ok {
		return nil, false
	}
	return core.advanceTimelineEffects(), true
}

func buildUtteranceChain(out aiOutput) []aiSegment {
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

func estimateWaitDuration(tts types.TTSEvent, waitSec float64) time.Duration {
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

func logRuntimeMessage(message string) {
	if strings.TrimSpace(message) == "" {
		return
	}
	log.Print(message)
}

func emitConversationSnapshotEffect(messages []types.ChatMessage) emitEventEffect {
	cloned := make([]types.ChatMessage, len(messages))
	copy(cloned, messages)
	return emitEventEffect{
		event: types.Event{
			Kind: types.EventConversationSnapshotUpdated,
			Payload: types.ConversationSnapshot{
				Messages: cloned,
			},
		},
	}
}

func emitConversationActivityEffect(at time.Time, source string) emitEventEffect {
	return emitEventEffect{
		event: types.Event{
			Kind: types.EventConversationActivity,
			Payload: types.ConversationActivity{
				At:     at,
				Source: source,
			},
		},
	}
}
