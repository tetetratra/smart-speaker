package types

import "encoding/json"

// EventKind represents the type of payload in an Event.
type EventKind int

const (
	EventTextInput EventKind = iota
	EventRealtimeOutput
	EventRealtimeAudio
	EventToolRequest
	EventToolResponse
	EventMCPCall
	EventResponsesRequest
	EventResponsesResponse
	EventTTSEnd
	EventReset
	EventSessionClear
	EventRTCSignal
)

// Event is the common data structure passed between stages.
type Event struct {
	Kind    EventKind
	Payload any
}

// ToolRequest は関数呼び出しが必要なときに発行されます。
type ToolRequest struct {
	ResponseID string
	ToolCallID string
	Name       string
	Arguments  json.RawMessage
}

// ToolResponse は実行結果を responses ステージに返します。
type ToolResponse struct {
	ToolCallID string
	Output     json.RawMessage
}
