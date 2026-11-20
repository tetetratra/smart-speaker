package conversation

import (
	"strings"
	"testing"
	"time"

	"github.com/tetetratra/smart-speaker/internal/types"
)

func TestRestoreKeepsAllRecordsAndMemory(t *testing.T) {
	records := []types.ConversationRecord{{ID: "old", Text: strings.Repeat("古い", 100)}, {ID: "new", Text: "最後の依頼"}}
	got := RestoreContext(records, []string{"長期メモリ"})
	if !strings.Contains(got, "古い") || !strings.Contains(got, "最後の依頼") || !strings.Contains(got, "- 長期メモリ") {
		t.Fatal(got)
	}
	// 変わりにくい長期メモリを先に置く。
	if strings.Index(got, "## 長期メモリ") > strings.Index(got, "## 過去の会話") {
		t.Fatalf("長期メモリが過去の会話より後ろにあります: %s", got)
	}
}

func TestRestoreWritesOneLinePerRecordWithoutInternalFields(t *testing.T) {
	at := time.Date(2026, 11, 20, 1, 15, 30, 0, time.UTC)
	records := []types.ConversationRecord{
		{ID: "3/item_a", GenerationID: 12, Role: types.RoleUser, Text: "今日の予定は？", Source: "realtime", CreatedAt: at},
		{ID: "3/tool/c", Role: types.RoleTool, Text: "list_events: {} → 歯医者", Source: "list_events", CallID: "c", CreatedAt: at},
		{ID: "x", Role: types.RoleBackground, Text: "調べた結果", Source: "web_search", CreatedAt: at},
	}
	got := RestoreContext(records, nil)
	for _, want := range []string{
		"[11/20 10:15] user: 今日の予定は？\n",
		"[11/20 10:15] tool: list_events: {} → 歯医者\n",
		"[11/20 10:15] background(web_search): 調べた結果\n",
		"- background:", // 凡例
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("%q がありません: %s", want, got)
		}
	}
	for _, unwanted := range []string{"item_a", "GenerationID", "## 長期メモリ"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("%q が含まれています: %s", unwanted, got)
		}
	}
}

func TestRestoreIsEmptyWithoutRecordsAndMemory(t *testing.T) {
	if got := RestoreContext(nil, nil); got != "" {
		t.Fatal(got)
	}
}

func TestRestoreDoesNotIncludeUnspokenGeneratedText(t *testing.T) {
	got := RestoreContext([]types.ConversationRecord{{Role: types.RoleAgent, Text: "実際に話した", Metadata: map[string]any{"generated_text": "まだ話していない"}}}, nil)
	if !strings.Contains(got, "実際に話した") || strings.Contains(got, "まだ話していない") {
		t.Fatal(got)
	}
}

func TestHistoryBytesCountsRestoredRecords(t *testing.T) {
	records := []types.ConversationRecord{{ID: "a", Text: "こんにちは", Metadata: map[string]any{"generated_text": strings.Repeat("長い", 100)}}, {ID: "b", Text: "ありがとう"}}
	_, lines, _ := strings.Cut(RestoreContext(records, nil), historyLegend+"\n")
	if got, want := HistoryBytes(records), len(lines); got != want {
		t.Fatalf("HistoryBytes() = %d, want %d", got, want)
	}
	if got := HistoryBytes(nil); got != 0 {
		t.Fatalf("HistoryBytes(nil) = %d, want 0", got)
	}
}
