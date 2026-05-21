package types

import "time"

// アシスタントやユーザーの 1 行分の出力
type OutputLine struct {
	Role       string
	Text       string
	ResponseID string
	Final      bool
	Source     string
}

// OutputAudio represents an assistant audio response chunk.
type OutputAudio struct {
	Role  string
	Audio string
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

// ChatMessage はResponses APIに渡す会話履歴の1メッセージです。
type ChatMessage struct {
	Role    string
	Content string
}

// ResponsesRequest はResponses APIへの要求を表します。
type ResponsesRequest struct {
	Role         string
	Text         string
	Messages     []ChatMessage
	RequestID    string
	SystemPrompt *string
}

// ResponsesStreamChunk はResponses API streamingから復元済みの1行、完了、エラーを表します。
type ResponsesStreamChunk struct {
	RequestID  string
	ResponseID string
	Line       string
	Done       bool
	Err        string
}
