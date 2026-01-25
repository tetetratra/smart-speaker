package types

import (
	"encoding/json"
	"fmt"
)

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

func (k EventKind) String() string {
	switch k {
	case EventTextInput:
		return "EventTextInput"
	case EventRealtimeOutput:
		return "EventRealtimeOutput"
	case EventRealtimeAudio:
		return "EventRealtimeAudio"
	case EventToolRequest:
		return "EventToolRequest"
	case EventToolResponse:
		return "EventToolResponse"
	case EventMCPCall:
		return "EventMCPCall"
	case EventResponsesRequest:
		return "EventResponsesRequest"
	case EventResponsesResponse:
		return "EventResponsesResponse"
	case EventTTSEnd:
		return "EventTTSEnd"
	case EventReset:
		return "EventReset"
	case EventSessionClear:
		return "EventSessionClear"
	case EventRTCSignal:
		return "EventRTCSignal"
	default:
		return fmt.Sprintf("EventKind(%d)", int(k))
	}
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
