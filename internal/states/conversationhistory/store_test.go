package conversationhistory

import (
	"encoding/json"
	"testing"

	types "smart-speaker/internal/types"
)

func TestStoreAppendSnapshotAndReset(t *testing.T) {
	store := NewStore()
	store.Append(types.ConversationRecord{Role: types.RoleUser, Text: "こんにちは", Metadata: map[string]any{"k": "v"}})
	store.Append(types.ConversationRecord{Role: types.RoleAssistant, Text: "はい"})

	snapshot := store.Snapshot()
	if len(snapshot) != 2 {
		t.Fatalf("Snapshot len = %d, want 2", len(snapshot))
	}
	snapshot[0].Text = "changed"
	snapshot[0].Metadata["k"] = "changed"

	again := store.Snapshot()
	if again[0].Text != "こんにちは" {
		t.Fatalf("record text changed through snapshot: %q", again[0].Text)
	}
	if again[0].Metadata["k"] != "v" {
		t.Fatalf("metadata changed through snapshot: %v", again[0].Metadata["k"])
	}

	store.Reset()
	if got := len(store.Snapshot()); got != 0 {
		t.Fatalf("Snapshot len after Reset = %d, want 0", got)
	}
}

func TestNewRecordMarksStaleToolResult(t *testing.T) {
	record := NewRecord(types.ConversationCommitRequest{
		ToolResult: &types.ToolResultRecord{
			ToolCallID:   "call-1",
			Name:         "get_temp",
			Output:       json.RawMessage(`{"temp":29}`),
			GenerationID: 1,
		},
	}, 2)

	if record.Role != types.RoleTool {
		t.Fatalf("Role = %s, want tool", record.Role)
	}
	if record.Metadata["stale"] != true {
		t.Fatalf("stale = %v, want true", record.Metadata["stale"])
	}
}
