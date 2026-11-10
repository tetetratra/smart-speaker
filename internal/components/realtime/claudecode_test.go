package realtime

import (
	"strings"
	"testing"

	"github.com/tetetratra/smart-speaker/internal/types"
)

// Claude Codeの通知は、ユーザーの発話ではなくアプリの差し込みとして会話履歴に残す。
func TestAgentPromptIsRecordedAsSystem(t *testing.T) {
	scheduled := make(chan types.ScheduledPrompt, 1)
	r := newRig(t, func(cfg *Config) { cfg.Scheduled = scheduled })
	scheduled <- types.ScheduledPrompt{ID: "agent/e1", Text: "[Claude Codeからの承認依頼]\n承認済みなので許可して", Source: "agent/claude-code/permission_request"}
	item := r.wait("conversation.item.create")["item"].(map[string]any)
	if text := item["content"].([]any)[0].(map[string]any)["text"].(string); !strings.HasPrefix(text, "[Claude Codeからの承認依頼]") {
		t.Fatal(text)
	}
	records := r.history.Snapshot()
	found := false
	for _, record := range records {
		if record.Role == types.RoleSystem && record.Source == "agent/claude-code/permission_request" {
			found = true
		}
	}
	if !found {
		t.Fatal(records)
	}
}

func TestClaudeCodeInstructionsOnlyWithTool(t *testing.T) {
	if got := withClaudeCodeInstructions("base", []any{map[string]any{"name": "web_search"}}); got != "base" {
		t.Fatal(got)
	}
	if got := withClaudeCodeInstructions("base", []any{map[string]any{"name": "ask_claude_code"}}); !strings.Contains(got, "Claude Codeの利用方針") {
		t.Fatal(got)
	}
}

// 後から届いたWeb検索の結果は、アプリが差し込んだ指示として会話履歴に残す。
func TestLateWebSearchResultIsMachineSource(t *testing.T) {
	if !isMachineSource("tool/web_search") {
		t.Fatal("後から届いた検索結果を、アプリが差し込んだ指示として扱っていません")
	}
}
