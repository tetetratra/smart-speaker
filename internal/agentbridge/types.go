// Package agentbridge は、手元のMacで動く agent-bridge（services/agent-bridge）とやり取りする。
// JSONの形は services/agent-bridge/src/protocol.ts と合わせる。
package agentbridge

import (
	"encoding/json"
	"time"
)

const (
	EventPermissionRequest = "permission_request"
	EventCompleted         = "completed"
	EventFailed            = "failed"
)

// Event はブリッジから届く通知。type によって使う項目が変わる。
type Event struct {
	Type      string `json:"type"`
	EventID   string `json:"event_id"`
	TaskID    string `json:"task_id"`
	SessionID string `json:"session_id"`

	// 承認依頼
	RequestID   string          `json:"request_id,omitempty"`
	Tool        string          `json:"tool,omitempty"`
	Title       string          `json:"title,omitempty"`
	Description string          `json:"description,omitempty"`
	Input       json.RawMessage `json:"input,omitempty"`
	ExpiresAt   time.Time       `json:"expires_at,omitempty"`

	// 完了・失敗
	Request           string   `json:"request,omitempty"`
	Summary           string   `json:"summary,omitempty"`
	Details           []string `json:"details,omitempty"`
	PermissionDenials []string `json:"permission_denials,omitempty"`
	Error             string   `json:"error,omitempty"`
}

type TaskAccepted struct {
	TaskID    string `json:"task_id"`
	SessionID string `json:"session_id"`
}

type SessionSummary struct {
	SessionID          string `json:"session_id"`
	Title              string `json:"title"`
	CustomTitle        string `json:"custom_title,omitempty"`
	FirstPrompt        string `json:"first_prompt,omitempty"`
	CreatedAt          string `json:"created_at,omitempty"`
	LastModified       string `json:"last_modified"`
	GitBranch          string `json:"git_branch,omitempty"`
	Tag                string `json:"tag,omitempty"`
	FromSmartSpeaker   bool   `json:"from_smart_speaker"`
	Running            bool   `json:"running"`
	TaskID             string `json:"task_id,omitempty"`
	PendingPermissions int    `json:"pending_permissions"`
}

type SessionTurn struct {
	Role  string   `json:"role"`
	Text  string   `json:"text"`
	Tools []string `json:"tools,omitempty"`
	At    string   `json:"at,omitempty"`
}

type SessionMessages struct {
	SessionID string        `json:"session_id"`
	Total     int           `json:"total"`
	Offset    int           `json:"offset"`
	Messages  []SessionTurn `json:"messages"`
}
