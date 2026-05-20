package conversation

import (
	"strings"

	types "smart-speaker/internal/types"
)

type signal interface {
	isSignal()
}

type humanTextSignal struct {
	text string
}
type responsesSignal struct {
	response types.ResponsesResponse
}
type responsesStreamChunkSignal struct {
	chunk types.ResponsesStreamChunk
}
type toolResponseSignal struct {
	response types.ToolResponse
}
type ttsEndSignal struct {
	event types.TTSEvent
}
type timerElapsedSignal struct{}

func (humanTextSignal) isSignal()            {}
func (responsesSignal) isSignal()            {}
func (responsesStreamChunkSignal) isSignal() {}
func (toolResponseSignal) isSignal()         {}
func (ttsEndSignal) isSignal()               {}
func (timerElapsedSignal) isSignal()         {}

func signalFromEvent(evt types.Event) (signal, bool) {
	switch evt.Kind {
	case types.EventHumanUtterance:
		line, ok := evt.Payload.(types.OutputLine)
		if !ok {
			return nil, false
		}
		text := strings.TrimSpace(line.Text)
		if text == "" {
			return nil, false
		}
		return humanTextSignal{text: text}, true
	case types.EventResponsesResponse:
		resp, ok := evt.Payload.(types.ResponsesResponse)
		if !ok {
			return nil, false
		}
		return responsesSignal{response: resp}, true
	case types.EventResponsesStreamChunk:
		chunk, ok := evt.Payload.(types.ResponsesStreamChunk)
		if !ok {
			return nil, false
		}
		return responsesStreamChunkSignal{chunk: chunk}, true
	case types.EventToolResponse:
		resp, ok := evt.Payload.(types.ToolResponse)
		if !ok {
			return nil, false
		}
		return toolResponseSignal{response: resp}, true
	case types.EventTTSEnd:
		tts, ok := evt.Payload.(types.TTSEvent)
		if !ok {
			return nil, false
		}
		return ttsEndSignal{event: tts}, true
	default:
		return nil, false
	}
}
