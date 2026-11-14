package types

import (
	"encoding/json"
	"reflect"
	"time"

	"github.com/pion/webrtc/v4/pkg/media"
)

// Event はイベントグラフを流れるもの。型がそのまま種類になる。
// イベントの型は Event を接頭辞にし、ほかでも使うデータの型を埋め込んで包む。
type Event interface{ isEvent() }

// EventName は転送ログなどに出すイベントの名前（型名）を返す。
func EventName(e Event) string {
	if e == nil {
		return "<nil>"
	}
	return reflect.TypeOf(e).Name()
}

// Is は e が T のイベントかどうかを返す。
func Is[T Event](e Event) bool {
	_, ok := e.(T)
	return ok
}

type EventSpeechStart struct{ SpeechEvent }
type EventSpeechEnd struct{ SpeechEvent }
type EventRealtimeOutput struct{ OutputLine }
type EventRealtimeStatus struct{ RealtimeStatus }
type EventToolRequest struct{ ToolRequest }
type EventRTCSignal struct{ RTCSignal }
type EventRTCPeerAudioFrame struct{ RTCPeerAudioFrame }
type EventRTCPeerOutputSink struct{ RTCPeerOutputSink }
type EventConversationCommitRequest struct{ ConversationCommitRequest }
type EventLLMRequest struct{ LLMRequest }
type EventResponseText struct{ ResponseText }
type EventGenerationChanged struct{ GenerationChanged }
type EventTimelineItem struct{ TimelineItem }
type EventPlayableSpeech struct{ PlayableSpeech }
type EventAgentTimelineEnd struct{ AgentTimelineEnd }

// EventAudioFrame は再生へ送る音声のフレーム。EventAudioSent は送り終えたフレームの通知。
type EventAudioFrame struct{ AudioFrame }
type EventAudioSent struct{ AudioFrame }

type EventLocalWakeUtterance struct{ LocalWakeUtterance }
type EventWakeTranscription struct{ WakeTranscription }

func (EventSpeechStart) isEvent()               {}
func (EventSpeechEnd) isEvent()                 {}
func (EventRealtimeOutput) isEvent()            {}
func (EventRealtimeStatus) isEvent()            {}
func (EventToolRequest) isEvent()               {}
func (EventRTCSignal) isEvent()                 {}
func (EventRTCPeerAudioFrame) isEvent()         {}
func (EventRTCPeerOutputSink) isEvent()         {}
func (EventConversationCommitRequest) isEvent() {}
func (EventLLMRequest) isEvent()                {}
func (EventResponseText) isEvent()              {}
func (EventGenerationChanged) isEvent()         {}
func (EventTimelineItem) isEvent()              {}
func (EventPlayableSpeech) isEvent()            {}
func (EventAgentTimelineEnd) isEvent()          {}
func (EventAudioFrame) isEvent()                {}
func (EventAudioSent) isEvent()                 {}
func (EventLocalWakeUtterance) isEvent()        {}
func (EventWakeTranscription) isEvent()         {}

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
