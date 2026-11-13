package conversation

import (
	"github.com/tetetratra/smart-speaker/internal/types"
	"strings"
	"testing"
)

func TestRestoreKeepsRecentRecordsAndMemoryWithoutChangingHistory(t *testing.T) {
	records := []types.ConversationRecord{{ID: "old", Text: strings.Repeat("古い", 100)}, {ID: "new", Text: "最後の依頼"}}
	got := RestoreContext(records, "長期メモリ", 300)
	if strings.Contains(got, "古い") || !strings.Contains(got, "最後の依頼") || !strings.Contains(got, "長期メモリ") {
		t.Fatal(got)
	}
	if !strings.Contains(records[0].Text, "古い") {
		t.Fatal("元の履歴を変更しました")
	}
}

func TestRestoreOmitsOrphanToolResult(t *testing.T) {
	records := []types.ConversationRecord{{Role: types.RoleToolCall, Text: strings.Repeat("呼び出し", 100)}, {Role: types.RoleToolResult, Text: "結果"}, {Role: types.RoleUser, Text: "次の依頼"}}
	got := RestoreContext(records, "", 300)
	if strings.Contains(got, "結果") || !strings.Contains(got, "次の依頼") {
		t.Fatal(got)
	}
}

func TestRestoreDoesNotIncludeUnspokenGeneratedText(t *testing.T) {
	got := RestoreContext([]types.ConversationRecord{{Role: types.RoleAgent, Text: "実際に話した", Metadata: map[string]any{"generated_text": "まだ話していない"}}}, "", 0)
	if !strings.Contains(got, "実際に話した") || strings.Contains(got, "まだ話していない") {
		t.Fatal(got)
	}
}
