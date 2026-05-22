package types

import (
	"encoding/json"
	"fmt"
)

// EventKind represents the type of payload in an Event.
type EventKind int

const (
	EventHumanUtterance EventKind = iota
	EventSpeechEnd
	EventRTCVADStatus
	EventRealtimeOutput
	EventRealtimeAudio
	EventToolRequest
	EventWhiteboardUpdate
	EventTTSEnd
	EventRTCSignal
	EventConversationCommitRequest
	EventLLMRequest
	EventTimelineItem
	EventPlayableSpeech
	EventScheduledItem
)

// Event is the common data structure passed between stages.
type Event struct {
	Kind    EventKind
	Payload any
}

func (k EventKind) String() string {
	switch k {
	case EventHumanUtterance:
		return "EventHumanUtterance"
	case EventSpeechEnd:
		return "EventSpeechEnd"
	case EventRTCVADStatus:
		return "EventRTCVADStatus"
	case EventRealtimeOutput:
		return "EventRealtimeOutput"
	case EventRealtimeAudio:
		return "EventRealtimeAudio"
	case EventToolRequest:
		return "EventToolRequest"
	case EventWhiteboardUpdate:
		return "EventWhiteboardUpdate"
	case EventTTSEnd:
		return "EventTTSEnd"
	case EventRTCSignal:
		return "EventRTCSignal"
	case EventConversationCommitRequest:
		return "EventConversationCommitRequest"
	case EventLLMRequest:
		return "EventLLMRequest"
	case EventTimelineItem:
		return "EventTimelineItem"
	case EventPlayableSpeech:
		return "EventPlayableSpeech"
	case EventScheduledItem:
		return "EventScheduledItem"
	default:
		return fmt.Sprintf("EventKind(%d)", int(k))
	}
}

// ToolRequest は関数呼び出しが必要なときに発行されます。
type ToolRequest struct {
	ResponseID   string
	ToolCallID   string
	Name         string
	Arguments    json.RawMessage
	GenerationID GenerationID
	SequenceID   string
}
