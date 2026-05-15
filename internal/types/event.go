package types

import (
	"encoding/json"
	"fmt"
)

// EventKind represents the type of payload in an Event.
type EventKind int

const (
	EventTextInput EventKind = iota
	EventSpeechStart
	EventSpeechEnd
	EventRTCVADStatus
	EventTimerFired
	EventConversationSnapshotUpdated
	EventConversationActivity
	EventRealtimeOutput
	EventRealtimeAudio
	EventToolRequest
	EventToolResponse
	EventResponsesRequest
	EventResponsesResponse
	EventResponsesStreamChunk
	EventWhiteboardUpdate
	EventTTSEnd
	EventTTSCancel
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
	case EventSpeechStart:
		return "EventSpeechStart"
	case EventSpeechEnd:
		return "EventSpeechEnd"
	case EventRTCVADStatus:
		return "EventRTCVADStatus"
	case EventTimerFired:
		return "EventTimerFired"
	case EventConversationSnapshotUpdated:
		return "EventConversationSnapshotUpdated"
	case EventConversationActivity:
		return "EventConversationActivity"
	case EventRealtimeOutput:
		return "EventRealtimeOutput"
	case EventRealtimeAudio:
		return "EventRealtimeAudio"
	case EventToolRequest:
		return "EventToolRequest"
	case EventToolResponse:
		return "EventToolResponse"
	case EventResponsesRequest:
		return "EventResponsesRequest"
	case EventResponsesResponse:
		return "EventResponsesResponse"
	case EventResponsesStreamChunk:
		return "EventResponsesStreamChunk"
	case EventWhiteboardUpdate:
		return "EventWhiteboardUpdate"
	case EventTTSEnd:
		return "EventTTSEnd"
	case EventTTSCancel:
		return "EventTTSCancel"
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
	Name       string
	ResponseID string
	Output     json.RawMessage
}

// TimerFiredEvent はタイマー発火時の通知イベントです。
type TimerFiredEvent struct {
	ReminderText string
}
