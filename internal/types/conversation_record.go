package types

import (
	"encoding/json"
	"time"
)

const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
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
}

// ToolResultRecord は tool 実行結果を履歴へ保存するための payload です。
type ToolResultRecord struct {
	ToolCallID          string
	Name                string
	Output              json.RawMessage
	GenerationID        GenerationID
	CurrentGenerationID GenerationID
	Stale               bool
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
	RequestID    string
	Role         string
	Text         string
	GenerationID GenerationID
}
