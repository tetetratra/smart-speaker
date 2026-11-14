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

// SpeechEvent は文字起こしの終了イベントを表します。
type SpeechEvent struct {
	Source     string
	CapturedAt time.Time
}

// AgentTimelineEnd は speechsegment が1つの応答の読み上げ単位をすべて発行し終えたことを表します。
type AgentTimelineEnd struct {
	ItemID       string
	GenerationID GenerationID
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
