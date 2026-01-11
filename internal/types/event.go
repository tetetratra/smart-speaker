package types

import "encoding/json"

// EventKind represents the type of payload in an Event.
type EventKind int

const (
	EventAudioChunk EventKind = iota
	EventTextInput
	EventRealtimeOutput
	EventRealtimeAudio
	EventToolRequest
	EventToolResponse
	EventMCPCall
	EventVadStart
	EventVadStop
	EventTranscriptPartial
	EventTranscriptFinal
	EventResponsesRequest
	EventResponsesResponse
	EventReset
	EventSessionClear
)

// Event is the common data structure passed between stages.
type Event struct {
	Kind    EventKind
	Payload any
}

// ToolRequest is emitted by the realtime stage when function calling is required.
type ToolRequest struct {
	ResponseID string
	ToolCallID string
	Name       string
	Arguments  json.RawMessage
}

// ToolResponse returns the execution result back to the realtime stage.
type ToolResponse struct {
	ToolCallID string
	Output     json.RawMessage
}
