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
	EventHumanUtterance EventKind = iota
	EventHumanInterimUtterance
	EventSpeechEnd
	EventRTCVADStatus
	EventRealtimeOutput
	EventRealtimeAudio
	EventToolRequest
	EventWhiteboardUpdate
	EventTTSEnd
	EventRTCSignal
	EventConversationCommitRequest
	EventSessionReset
	EventLLMRequest
	EventTimelineItem
	EventPlayableSpeech
	EventScheduledItem
	EventAgentTimelineEnd
	EventAgentSpeechPlaybackEnd
	EventRTCPeerAudioFrame
	EventRTCSpeechAudio
	EventRTCPeerOutputSink
	EventTimerState
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
	case EventHumanInterimUtterance:
		return "EventHumanInterimUtterance"
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
	case EventSessionReset:
		return "EventSessionReset"
	case EventLLMRequest:
		return "EventLLMRequest"
	case EventTimelineItem:
		return "EventTimelineItem"
	case EventPlayableSpeech:
		return "EventPlayableSpeech"
	case EventScheduledItem:
		return "EventScheduledItem"
	case EventAgentTimelineEnd:
		return "EventAgentTimelineEnd"
	case EventAgentSpeechPlaybackEnd:
		return "EventAgentSpeechPlaybackEnd"
	case EventRTCPeerAudioFrame:
		return "EventRTCPeerAudioFrame"
	case EventRTCSpeechAudio:
		return "EventRTCSpeechAudio"
	case EventRTCPeerOutputSink:
		return "EventRTCPeerOutputSink"
	case EventTimerState:
		return "EventTimerState"
	default:
		return fmt.Sprintf("EventKind(%d)", int(k))
	}
}

const (
	RTCSpeechAudioStart = "start"
	RTCSpeechAudioFrame = "audio"
	RTCSpeechAudioEnd   = "end"
)

type RTCPeerAudioFrame struct {
	PeerID     string
	Samples    []int16
	PCM        []byte
	SampleRate int
	Channels   int
	DurationMs int
	CapturedAt time.Time
}

type RTCSpeechAudio struct {
	PeerID     string
	Type       string
	PCM        []byte
	Prebuffer  []byte
	SampleRate int
	Channels   int
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
