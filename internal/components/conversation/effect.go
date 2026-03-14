package conversation

import (
	"time"

	types "smart-speaker/internal/types"
)

type effect interface {
	isEffect()
}

type emitEventEffect struct {
	event types.Event
}

type startTimerEffect struct {
	duration time.Duration
}

type stopTimerEffect struct{}

type requestResponseEffect struct {
	requestID string
	messages  []types.ChatMessage
	tools     []any
}

type logRecordEffect struct {
	record logRecord
}

type markActivityEffect struct {
	at time.Time
}

type updateConversationStateEffect struct {
	messages []types.ChatMessage
}

type clearConversationStateEffect struct{}

type invalidateCalendarContextEffect struct{}

type clearContextsEffect struct{}

type runtimeLogEffect struct {
	message string
}

func (emitEventEffect) isEffect()                 {}
func (startTimerEffect) isEffect()                {}
func (stopTimerEffect) isEffect()                 {}
func (requestResponseEffect) isEffect()           {}
func (logRecordEffect) isEffect()                 {}
func (markActivityEffect) isEffect()              {}
func (updateConversationStateEffect) isEffect()   {}
func (clearConversationStateEffect) isEffect()    {}
func (invalidateCalendarContextEffect) isEffect() {}
func (clearContextsEffect) isEffect()             {}
func (runtimeLogEffect) isEffect()                {}
