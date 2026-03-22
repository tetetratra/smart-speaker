package conversation

import (
	"regexp"
	"strings"

	types "smart-speaker/internal/types"
)

var (
	markdownLinkPattern = regexp.MustCompile(`\[[^\]]*\]\((https?://[^)]+)\)`)
	bareURLPattern      = regexp.MustCompile(`https?://\S+`)
	citationPattern     = regexp.MustCompile("cite[^]+")
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
	root := buildTimelineSegments(out)
	if len(root) == 0 {
		return effects, true
	}
	core.state.pendingTimeline = root
	core.state.pendingTimelineIdx = 0
	effects = append(effects, core.advanceTimelineEffects()...)
	return effects, true
}

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
