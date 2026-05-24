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

func TestNewRecordStoresToolCall(t *testing.T) {
	record := NewRecord(types.ConversationCommitRequest{
		ToolCall: &types.ToolCallRecord{
			ToolCallID:   "call-1",
			Name:         "web_search",
			Arguments:    json.RawMessage(`{"query":"東京 天気"}`),
			GenerationID: 3,
		},
	}, 3)

	if record.Role != types.RoleToolCall {
		t.Fatalf("Role = %s, want tool_call", record.Role)
	}
	if record.Source != "web_search" {
		t.Fatalf("Source = %q, want web_search", record.Source)
	}
	if record.Text != `{"query":"東京 天気"}` {
		t.Fatalf("Text = %q, want args json", record.Text)
	}
	if record.Metadata["tool_call_id"] != "call-1" {
		t.Fatalf("tool_call_id = %v, want call-1", record.Metadata["tool_call_id"])
	}
	if record.Metadata["tool_name"] != "web_search" {
		t.Fatalf("tool_name = %v, want web_search", record.Metadata["tool_name"])
	}
}
