package types

import (
	"encoding/json"
	"time"
)

const (
	RoleUser       = "user"
	RoleAgent      = "agent"
	RoleSystem     = "system"
	RoleToolCall   = "tool_call"
	RoleToolResult = "tool_result"
)

// ConversationRecord は LLM に渡す会話履歴の1件です。
type ConversationRecord struct {
	ID           string
	Role         string
	Text         string
	GenerationID GenerationID
	Source       string
	Metadata     map[string]any
	CreatedAt    time.Time
	// 以下は画面の表示用。Text はモデル向けに今の形のまま残し、同じ内容を重ねて持つ。
	// 古い履歴ファイルには無い項目なので、空なら保存しない。
	Title     string `json:",omitempty"`
	External  bool   `json:",omitempty"`
	ToolName  string `json:",omitempty"`
	CallID    string `json:",omitempty"`
	Arguments string `json:",omitempty"`
	Output    string `json:",omitempty"`
}

// IsDialogue は、ユーザーとモデルの発話かどうかを返す。ツールや通知の記録は false。
func (r ConversationRecord) IsDialogue() bool {
	return r.Role == RoleUser || r.Role == RoleAgent
}

// ToolResultRecord は tool 実行結果を履歴へ保存するための payload です。
type ToolResultRecord struct {
	ToolCallID   string
	Name         string
	Output       json.RawMessage
	GenerationID GenerationID
}

// ConversationCommitRequest は会話履歴への保存要求です。
type ConversationCommitRequest struct {
	Role         string
	Text         string
	GenerationID GenerationID
	Source       string
	ToolResult   *ToolResultRecord
}

// LLMRequest は LLM component への推論要求です。
type LLMRequest struct {
	Title    string
	Metadata map[string]any
	// Inbound は差し込み専用経路からの入力。接続待ちの間も保持する。
	Inbound      bool
	Source       string
	RequestID    string
	Role         string
	Text         string
	GenerationID GenerationID
}
