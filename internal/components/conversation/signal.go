package conversation

import (
	"strings"

	types "smart-speaker/internal/types"
)

type signal interface {
	isSignal()
}

type speechStartSignal struct{}
type humanTextSignal struct {
	text string
}
type timerFiredSignal struct {
	event types.TimerFiredEvent
}
type responsesSignal struct {
	response types.ResponsesResponse
}
type toolResponseSignal struct {
	response types.ToolResponse
}
type sessionClearSignal struct{}
type ttsEndSignal struct {
	event types.TTSEvent
}
type timerElapsedSignal struct{}

func (speechStartSignal) isSignal()  {}
func (humanTextSignal) isSignal()    {}
func (timerFiredSignal) isSignal()   {}
func (responsesSignal) isSignal()    {}
func (toolResponseSignal) isSignal() {}
func (sessionClearSignal) isSignal() {}
func (ttsEndSignal) isSignal()       {}
func (timerElapsedSignal) isSignal() {}

func signalFromEvent(evt types.Event) (signal, bool) {
	switch evt.Kind {
	case types.EventSpeechStart:
		return speechStartSignal{}, true
	case types.EventTextInput:
		line, ok := evt.Payload.(types.OutputLine)
		if !ok {
			return nil, false
		}
		text := strings.TrimSpace(line.Text)
		if text == "" {
			return nil, false
		}
		return humanTextSignal{text: text}, true
	case types.EventTimerFired:
		timerEvt, ok := evt.Payload.(types.TimerFiredEvent)
		if !ok {
			return nil, false
		}
		return timerFiredSignal{event: timerEvt}, true
	case types.EventResponsesResponse:
		resp, ok := evt.Payload.(types.ResponsesResponse)
		if !ok {
			return nil, false
		}
		return responsesSignal{response: resp}, true
	case types.EventToolResponse:
		resp, ok := evt.Payload.(types.ToolResponse)
		if !ok {
			return nil, false
		}
		return toolResponseSignal{response: resp}, true
	case types.EventSessionClear:
		return sessionClearSignal{}, true
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
