package types

import "time"

const (
	EventResponseText EventKind = 100 + iota
	EventGenerationChanged
	EventAudioFrame
	EventAudioSent
	EventRealtimeStatus
)

// ResponseText はAPI応答の差分。識別子は接続更新後も別応答と混ざらない。
type ResponseText struct {
	GenerationID GenerationID
	ResponseID   string
	ItemID       string
	Text         string
	Done         bool
}

type GenerationChanged struct {
	GenerationID GenerationID
	ItemID       string
}
type AudioFrame struct {
	ItemID       string        `json:"item_id,omitempty"`
	GenerationID GenerationID  `json:"generation_id"`
	SegmentID    string        `json:"segment_id"`
	Sequence     int           `json:"sequence"`
	Audio        string        `json:"audio"`
	SampleRate   int           `json:"sample_rate"`
	Text         string        `json:"text,omitempty"`
	Final        bool          `json:"final"`
	End          bool          `json:"end,omitempty"`
	Duration     time.Duration `json:"-"`
	// SegmentDuration は読み上げ区間全体の再生時間。区間の先頭フレームだけに設定する。
	SegmentDuration time.Duration `json:"-"`
}
type RealtimeStatus struct {
	Status  string
	Message string
}

// EventGeneration は応答出力の世代だけを取り出す。履歴・ツール結果は破棄対象にしない。
func EventGeneration(e Event) (uint64, bool) {
	switch p := e.Payload.(type) {
	case ResponseText:
		return uint64(p.GenerationID), true
	case ToolRequest:
		return uint64(p.GenerationID), true
	case TimelineItem:
		return uint64(p.GenerationID), true
	case PlayableSpeech:
		return uint64(p.GenerationID), true
	case AudioFrame:
		return uint64(p.GenerationID), true
	case AgentTimelineEnd:
		return uint64(p.GenerationID), true
	default:
		return 0, false
	}
}

const EventLocalWakeUtterance EventKind = 120

// LocalWakeUtteranceはローカルの認識・呼びかけ判定を通過した1回の発話。
type LocalWakeUtterance struct {
	PeerID, Text string
	Revision     uint64
	Confidence   float64
	// WakeOnlyは呼びかけだけで依頼の本文がない発話。短い返事だけを返す。
	WakeOnly bool
}

// ScheduledPrompt は登録済みの自然言語ジョブを会話処理へ渡す。
type ScheduledPrompt struct {
	ID, Name, Instruction string
	Speak                 bool
}
