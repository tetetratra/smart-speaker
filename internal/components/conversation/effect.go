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
	requestID    string
	generationID uint64
	messages     []types.ChatMessage
}

type logRecordEffect struct {
	record logRecord
}

type runtimeLogEffect struct {
	message string
}

func (emitEventEffect) isEffect()       {}
func (startTimerEffect) isEffect()      {}
func (stopTimerEffect) isEffect()       {}
func (requestResponseEffect) isEffect() {}
func (logRecordEffect) isEffect()       {}
func (runtimeLogEffect) isEffect()      {}
