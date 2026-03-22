package conversation

import (
	"strings"

	types "smart-speaker/internal/types"
)

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
