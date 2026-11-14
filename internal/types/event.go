package types

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/pion/webrtc/v4/pkg/media"
)

// EventKind represents the type of payload in an Event.
type EventKind int

const (
	EventSpeechStart EventKind = iota
	EventSpeechEnd
	EventRealtimeOutput
	EventToolRequest
	EventRTCSignal
	EventConversationCommitRequest
	EventLLMRequest
	EventTimelineItem
	EventPlayableSpeech
	EventAgentTimelineEnd
	EventRTCPeerAudioFrame
	EventRTCPeerOutputSink
)

// Event is the common data structure passed between stages.
type Event struct {
	Kind    EventKind
	Payload any
}

func (k EventKind) String() string {
	switch k {
	case EventConversationInput:
		return "EventConversationInput"
	case EventConversationSignal:
		return "EventConversationSignal"
	case EventLocalWakeUtterance:
		return "EventLocalWakeUtterance"
	case EventWakeTranscription:
		return "EventWakeTranscription"
	case EventResponseText:
		return "EventResponseText"
	case EventGenerationChanged:
		return "EventGenerationChanged"
	case EventAudioFrame:
		return "EventAudioFrame"
	case EventAudioSent:
		return "EventAudioSent"
	case EventRealtimeStatus:
		return "EventRealtimeStatus"

	case EventSpeechStart:
		return "EventSpeechStart"
	case EventSpeechEnd:
		return "EventSpeechEnd"
	case EventRealtimeOutput:
		return "EventRealtimeOutput"
	case EventToolRequest:
		return "EventToolRequest"
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
	case EventAgentTimelineEnd:
		return "EventAgentTimelineEnd"
	case EventRTCPeerAudioFrame:
		return "EventRTCPeerAudioFrame"
	case EventRTCPeerOutputSink:
		return "EventRTCPeerOutputSink"
	default:
		return fmt.Sprintf("EventKind(%d)", int(k))
	}
}

type RTCPeerAudioFrame struct {
	PeerID     string
	Samples    []int16
	PCM        []byte
	SampleRate int
	Channels   int
	DurationMs int
	CapturedAt time.Time
}

type RTCPeerOutputWriter interface {
	WriteSample(sample media.Sample) error
}

type RTCPeerOutputSink struct {
	PeerID       string
	Writer       RTCPeerOutputWriter
	OpusChannels int
	Connected    bool
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
