package types

import "time"

// アシスタントやユーザーの 1 行分の出力
type OutputLine struct {
	Role         string
	Text         string
	ResponseID   string
	Final        bool
	Source       string
	GenerationID GenerationID
}

// OutputAudio represents an agent audio response chunk.
type OutputAudio struct {
	Role         string
	Audio        string
	Text         string
	GenerationID GenerationID
}

// SpeechEvent は文字起こしの終了イベントを表します。
type SpeechEvent struct {
	Source     string
	CapturedAt time.Time
}

// RTCVADStatus はサーバー側VADの現在音量としきい値を表します。
type RTCVADStatus struct {
	InputLevel int
	Threshold  int
	CapturedAt time.Time
}

// TTSEvent はTTSストリーム完了を表します。
type TTSEvent struct {
	ResponseID      string
	AudioStartAt    time.Time
	DurationSeconds float64
}

// WhiteboardUpdate はアプリ画面の白板表示更新を表します。
type WhiteboardUpdate struct {
	Content string
}

type RTCIceCandidate struct {
	Candidate     string  `json:"candidate"`
	SDPMid        *string `json:"sdpMid,omitempty"`
	SDPMLineIndex *uint16 `json:"sdpMLineIndex,omitempty"`
}

type RTCSignal struct {
	Type      string           `json:"type"`
	SDP       string           `json:"sdp,omitempty"`
	Candidate *RTCIceCandidate `json:"candidate,omitempty"`
	ClientID  string           `json:"-"`
}

// ChatMessage は LLM component 内で扱う正規化済み会話履歴です。
// Role はアプリ内の user / agent / tool_call / tool_result を保持し、
// Responses API の外部 role への変換は HTTP payload 作成直前に行います。
type ChatMessage struct {
	Role    string
	Content string
}
