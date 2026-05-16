package conversation

import (
	"strings"

	types "smart-speaker/internal/types"
)

type responsesStreamRule struct{}

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
	parsedChunks, ok := parseAIChunks(line)
	if !ok {
		return core.failStream("conversation: invalid stream chunk: "+line, true), true
	}

	var effects []effect
	shouldAdvance := false
	for _, parsed := range parsedChunks {
		switch parsed.Type {
		case "speech":
			text := sanitizeSpeech(parsed.Text)
			if text == "" {
				return core.failStream("conversation: invalid stream speech after sanitize: "+line, true), true
			}
			core.state.pendingStreamSpeechStarted = true
			core.state.pendingTimeline = append(core.state.pendingTimeline, timelineSegment{Type: "speech", Text: text})
			shouldAdvance = true
		case "wait":
			core.state.pendingTimeline = append(core.state.pendingTimeline, timelineSegment{
				Type:    "wait",
				WaitSec: sanitizeWait(parsed.Sec),
			})
			shouldAdvance = true
		case "whiteboard":
			effects = append(effects, emitEventEffect{
				event: types.Event{
					Kind: types.EventWhiteboardUpdate,
					Payload: types.WhiteboardUpdate{
						Content: parsed.Content,
					},
				},
			})
		default:
			return core.failStream("conversation: invalid stream chunk: "+line, true), true
		}
	}
	if shouldAdvance && core.state.current == nil && !core.state.pendingTimelineTimerWaiting {
		effects = append(effects, core.advanceTimelineEffects()...)
	}
	return effects, true
}

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
