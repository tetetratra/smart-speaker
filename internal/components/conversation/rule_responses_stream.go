package conversation

import "strings"

type responsesStreamRule struct{}

func (responsesStreamRule) Apply(core *conversationCore, sig signal) ([]effect, bool) {
	s, ok := sig.(responsesStreamChunkSignal)
	if !ok {
		return nil, false
	}
	chunk := s.chunk
	generationID, ok := core.state.requestGeneration[chunk.RequestID]
	if chunk.RequestID == "" || !ok {
		return nil, true
	}
	if generationID != core.state.currentGeneration() {
		return nil, true
	}
	core.state.pendingRequestStreaming = true
	if strings.TrimSpace(chunk.Err) != "" {
		return core.failStream(chunk.Err, false, chunk.Err), true
	}
	if chunk.Done {
		return core.completeStream(), true
	}
	if core.state.pendingStreamFailed {
		return nil, true
	}
	line := strings.TrimSpace(chunk.Line)
	if line != "" {
		core.state.pendingStreamLines = append(core.state.pendingStreamLines, line)
	}
	parsedChunks, ok := parseAIChunks(line)
	if !ok {
		return core.failStream("conversation: invalid stream chunk: "+line, true, line), true
	}

	var effects []effect
	shouldAdvance := false
	for _, parsed := range parsedChunks {
		if core.state.pendingStreamToolSeen {
			return core.failStream("conversation: stream chunk after tool is not allowed: "+line, true, line), true
		}
		switch parsed.Type {
		case "speech":
			text := sanitizeSpeech(parsed.Text)
			if text == "" {
				return core.failStream("conversation: invalid stream speech after sanitize: "+line, true, line), true
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
		case "tool":
			core.state.pendingStreamToolSeen = true
			core.state.pendingTimeline = append(core.state.pendingTimeline, timelineSegment{
				Type: "tool",
				Tool: &toolCallSegment{
					Name: parsed.Name,
					Args: parsed.Args,
				},
			})
			shouldAdvance = true
		default:
			return core.failStream("conversation: invalid stream chunk: "+line, true, line), true
		}
	}
	if shouldAdvance && core.state.current == nil && !core.state.pendingTimelineTimerWaiting {
		effects = append(effects, core.advanceTimelineEffects()...)
	}
	return effects, true
}

func (c *conversationCore) failStream(message string, retryInvalid bool, invalidRaw string) []effect {
	effects := []effect{runtimeLogEffect{message: message}}
	c.state.pendingStreamFailed = true
	c.state.pendingRequestStreaming = false
	delete(c.state.requestGeneration, c.state.pendingRequestID)
	if c.state.pendingStreamSpeechStarted {
		c.state.pendingRequestID = ""
		c.state.clearPendingTimeline()
		c.state.clearPendingStreamLines()
		return effects
	}
	c.state.pendingRequestID = ""
	c.state.clearPendingTimeline()
	if retryInvalid {
		effects = append(effects, c.retryInvalidResponseEffects(invalidRaw)...)
	}
	c.state.clearPendingStreamLines()
	return effects
}

func (c *conversationCore) completeStream() []effect {
	c.state.pendingRequestStreaming = false
	delete(c.state.requestGeneration, c.state.pendingRequestID)
	c.state.pendingRequestID = ""
	if !c.state.pendingStreamSpeechStarted && !c.state.pendingStreamToolSeen {
		invalidRaw := strings.Join(c.state.pendingStreamLines, "\n")
		c.state.clearPendingTimeline()
		effects := []effect{runtimeLogEffect{
			message: "conversation: invalid stream response: no speech chunk",
		}}
		effects = append(effects, c.retryInvalidResponseEffects(invalidRaw)...)
		c.state.clearPendingStreamLines()
		return effects
	}
	c.state.invalidResponseRetries = 0
	c.state.clearPendingStreamLines()
	if c.state.current != nil || c.state.hasPendingSpeech() || c.state.pendingTimelineTimerWaiting {
		return nil
	}
	c.state.clearPendingTimeline()
	return nil
}
