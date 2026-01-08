package types

import "encoding/json"

// Base64 で表現された PCM チャンク
type AudioChunk string

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

// TranscriptEvent は文字起こしイベントを表します。
type TranscriptEvent struct {
	Text  string
	Final bool
}

// VadEvent はVADの開始/終了を表します。
type VadEvent struct {
	Type string // "start" または "stop"
}

// ResponsesRequest はResponses APIへの要求を表します。
type ResponsesRequest struct {
	Role       string
	Text       string
	ToolChoice any
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
