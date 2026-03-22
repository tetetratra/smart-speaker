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
