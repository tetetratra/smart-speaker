package conversationhistory

import (
	"encoding/json"
	"testing"

	types "github.com/tetetratra/smart-speaker/internal/types"
)

func TestStoreAppendSnapshotAndReset(t *testing.T) {
	store := NewStore()
	store.Append(types.ConversationRecord{Role: types.RoleUser, Text: "こんにちは", Metadata: map[string]any{"k": "v"}})
	store.Append(types.ConversationRecord{Role: types.RoleAgent, Text: "はい"})

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

	if record.Role != types.RoleToolResult {
		t.Fatalf("Role = %s, want tool_result", record.Role)
	}
	if record.Metadata["stale"] != true {
		t.Fatalf("stale = %v, want true", record.Metadata["stale"])
	}
}

func TestNewRecordCreatesToolCall(t *testing.T) {
	record := NewRecord(types.ConversationCommitRequest{
		ToolCall: &types.ToolCallRecord{
			ToolCallID:   "call-1",
			Name:         "get_temp",
			Arguments:    json.RawMessage(`{"place":"living"}`),
			GenerationID: 1,
		},
	}, 1)

	if record.Role != types.RoleToolCall {
		t.Fatalf("Role = %s, want tool_call", record.Role)
	}
	if record.Text != `{"place":"living"}` {
		t.Fatalf("Text = %q, want arguments JSON", record.Text)
	}
	if record.Metadata["tool_call_id"] != "call-1" {
		t.Fatalf("tool_call_id = %v, want call-1", record.Metadata["tool_call_id"])
	}
}

func TestToChatMessagesKeepsNewRolesWithJSONContent(t *testing.T) {
	messages := ToChatMessages([]types.ConversationRecord{
		{Role: types.RoleUser, Text: "温度見て", Source: "server-stt", GenerationID: 1},
		{Role: types.RoleAgent, Text: "確認します", Source: "llm", GenerationID: 1},
		{Role: types.RoleToolCall, Text: `{"place":"living"}`, Source: "get_temp", GenerationID: 1, Metadata: map[string]any{"tool_call_id": "call-1"}},
		{Role: types.RoleToolResult, Text: `{"temp":29}`, Source: "get_temp", GenerationID: 1, Metadata: map[string]any{"tool_call_id": "call-1", "stale": false}},
	})

	if len(messages) != 4 {
		t.Fatalf("len = %d, want 4", len(messages))
	}
	for i, want := range []string{types.RoleUser, types.RoleAgent, types.RoleToolCall, types.RoleToolResult} {
		if messages[i].Role != want {
			t.Fatalf("messages[%d].Role = %s, want %s", i, messages[i].Role, want)
		}
	}

	var call map[string]any
	if err := json.Unmarshal([]byte(messages[2].Content), &call); err != nil {
		t.Fatalf("tool call content JSON: %v", err)
	}
	if call["type"] != "tool_call" || call["tool_call_id"] != "call-1" {
		t.Fatalf("tool call content = %#v", call)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(messages[3].Content), &result); err != nil {
		t.Fatalf("tool result content JSON: %v", err)
	}
	if result["type"] != "tool_result" || result["tool_call_id"] != "call-1" {
		t.Fatalf("tool result content = %#v", result)
	}
}
