package types

import (
	"encoding/json"
	"time"
)

const (
	// RoleUser はユーザーの発話・入力。
	RoleUser = "user"
	// RoleAgent はモデルの発話。
	RoleAgent = "agent"
	// RoleTool は、実行し終えたツール1回分（呼び出しと結果）。
	RoleTool = "tool"
	// RoleBackground は、頼んだ処理（Web 検索、Claude Code など）から、あとで届く結果や知らせ。
	RoleBackground = "background"
	// RoleSystem は、アプリ自身からの知らせ（予約ジョブの remind・失敗、発話が遮られた記録、居場所の変化など）。
	RoleSystem = "system"
)

// Role は送り元の種類で決める。予約ジョブから呼ばれたか（history_only）では変えない。
// 古い履歴ファイルには tool_call / tool_result の記録が残っているが、変換はしない。

// ConversationRecord は LLM に渡す会話履歴の1件です。
type ConversationRecord struct {
	ID           string
	Role         string
	Text         string
	GenerationID GenerationID
	// Source は記録の中身を作った機能。ツールの記録ならツール名、それ以外は automation・web_search・claude_code・realtime など。
	// ツール名に合わせて、スネークケースの1語にする。
	Source    string
	Metadata  map[string]any
	CreatedAt time.Time
	// 以下は画面の表示用。Text はモデル向けに今の形のまま残し、同じ内容を重ねて持つ。
	// 古い履歴ファイルには無い項目なので、空なら保存しない。
	// Title は何についての記録か（予定の件名、検索のクエリ、予約ジョブの名前など）。件名がなければ空。
	Title     string `json:",omitempty"`
	External  bool   `json:",omitempty"`
	CallID    string `json:",omitempty"`
	Arguments string `json:",omitempty"`
	Output    string `json:",omitempty"`
	// Error は失敗の内容。空でなければ、Role によらず失敗として画面に出す。
	Error string `json:",omitempty"`
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
