package conversation

import (
	"strings"

	types "smart-speaker/internal/types"
)

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
