package types

// EventKind represents the type of payload in an Event.
type EventKind int

const (
	EventAudioChunk EventKind = iota
	EventRealtimeOutput
	EventToolRequest
	EventToolResponse
)

// Event is the common data structure passed between stages.
type Event struct {
	Kind    EventKind
	Payload any
}
