package types

import (
	"time"
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
	switch p := e.(type) {
	case EventResponseText:
		return uint64(p.GenerationID), true
	case EventToolRequest:
		return uint64(p.GenerationID), true
	case EventTimelineItem:
		return uint64(p.GenerationID), true
	case EventPlayableSpeech:
		return uint64(p.GenerationID), true
	case EventAudioFrame:
		return uint64(p.GenerationID), true
	case EventAudioSent:
		return uint64(p.GenerationID), true
	case EventAgentTimelineEnd:
		return uint64(p.GenerationID), true
	default:
		return 0, false
	}
}

// LocalWakeUtteranceはローカルの認識・呼びかけ判定を通過した1回の発話。
type LocalWakeUtterance struct {
	PeerID, Text string
	Revision     uint64
	Confidence   float64
	// WakeOnlyは呼びかけだけで依頼の本文がない発話。起きていることを短く伝える応答にする。
	WakeOnly bool
}

// WakeTranscriptionの結果の種類。
const (
	// WakeTranscriptionJudgedは呼びかけを判定した結果。受理・拒否の両方を含む。
	WakeTranscriptionJudged = "judged"
	// WakeTranscriptionDiscardedReconnectは端末の再接続で、判定せずに捨てた認識結果。
	WakeTranscriptionDiscardedReconnect = "discarded_reconnect"
	// WakeTranscriptionDiscardedModeChangeは設定の変更で、判定せずに捨てた認識結果。
	WakeTranscriptionDiscardedModeChange = "discarded_mode_change"
)

// WakeTranscriptionは呼びかけ必須モードでローカルSTTが書き起こした1回の発話と、その扱い。
// 呼びかけの見逃しを調べるため、拒否・破棄した発話も含めて画面のログへ送る。
type WakeTranscription struct {
	PeerID, Text string
	Outcome      string
	// Confidence以降は判定した結果（Outcomeがjudgedのとき）。破棄したときはConfidenceにSTTの信頼度だけを入れる。
	Confidence, Similarity      float64
	Matched, Accepted, WakeOnly bool
	// Awakeは判定の時点で会話中だったか。
	Awake bool
	// ErrorはSTTの失敗内容。成功したときは空。
	Error string
}
