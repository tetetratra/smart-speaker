package conversation

import (
	"strings"
	"testing"

	"github.com/tetetratra/smart-speaker/internal/types"
)

func TestRestoreKeepsAllRecordsAndMemory(t *testing.T) {
	records := []types.ConversationRecord{{ID: "old", Text: strings.Repeat("古い", 100)}, {ID: "new", Text: "最後の依頼"}}
	got := RestoreContext(records, "長期メモリ")
	if !strings.Contains(got, "古い") || !strings.Contains(got, "最後の依頼") || !strings.Contains(got, "長期メモリ") {
		t.Fatal(got)
	}
}

func TestRestoreDoesNotIncludeUnspokenGeneratedText(t *testing.T) {
	got := RestoreContext([]types.ConversationRecord{{Role: types.RoleAgent, Text: "実際に話した", Metadata: map[string]any{"generated_text": "まだ話していない"}}}, "")
	if !strings.Contains(got, "実際に話した") || strings.Contains(got, "まだ話していない") {
		t.Fatal(got)
	}
}

func TestHistoryBytesCountsRestoredRecords(t *testing.T) {
	records := []types.ConversationRecord{{ID: "a", Text: "こんにちは", Metadata: map[string]any{"generated_text": strings.Repeat("長い", 100)}}, {ID: "b", Text: "ありがとう"}}
	if got, want := HistoryBytes(records), len(RestoreContext(records, "")); got != want {
		t.Fatalf("HistoryBytes() = %d, want %d", got, want)
	}
	if got := HistoryBytes(nil); got != 0 {
		t.Fatalf("HistoryBytes(nil) = %d, want 0", got)
	}
}
