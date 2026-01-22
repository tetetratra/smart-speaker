package types

import "encoding/json"

// アシスタントやユーザーの 1 行分の出力
type OutputLine struct {
	Role        string
	Text        string
	ResponseID  string
	Final       bool
	Source      string
	Expectation *int
}

// OutputAudio represents an assistant audio response chunk.
type OutputAudio struct {
	Role  string
	Audio string
}

// TTSEvent はTTSストリーム完了を表します。
type TTSEvent struct {
	ResponseID string
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
}

// ResponsesRequest はResponses APIへの要求を表します。
type ResponsesRequest struct {
	Role       string
	Text       string
	ToolChoice any
	Tools      []any
}

// ResponsesResponse はResponses APIの応答を表します。
type ResponsesResponse struct {
	Text        string
	ResponseID  string
	HasResponse bool
	ToolCalls   []ToolRequest
	MCPCalls    []MCPCall
}

// MCPCall はResponses APIのMCP呼び出し結果を表します。
type MCPCall struct {
	CallID      string
	ServerLabel string
	Name        string
	Arguments   json.RawMessage
	Output      json.RawMessage
	ResponseID  string
}
