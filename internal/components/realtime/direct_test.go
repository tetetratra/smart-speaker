package realtime

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/tetetratra/smart-speaker/internal/types"
)

func TestDirectToolCallIsRecordedWithoutResponse(t *testing.T) {
	direct := make(chan types.DirectToolCall, 1)
	r := newRig(t, func(cfg *Config) { cfg.DirectToolCalls = direct })
	// 接続済みのセッションを用意する。
	r.node.Upstream <- types.Event{Payload: types.LLMRequest{Text: "こんにちは"}}
	r.wait("conversation.item.create")
	req := r.wait("response.create")
	r.response("hello-response", "hello-item", "こんにちは。", req["response"].(map[string]any)["metadata"])
	r.event(types.EventResponseText)

	direct <- types.DirectToolCall{
		Source:  "automation/job1",
		Request: types.ToolRequest{ToolCallID: "automation_1", Name: "set_wake_mode", Arguments: json.RawMessage(`{"required":true}`)},
		Result:  types.ToolResultRecord{ToolCallID: "automation_1", Name: "set_wake_mode", Output: json.RawMessage(`{"required":true}`)},
	}
	item := r.wait("conversation.item.create")["item"].(map[string]any)
	text := item["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "過去の操作の実行結果") || !strings.Contains(text, "set_wake_mode") {
		t.Fatal(text)
	}
	deadline := time.After(50 * time.Millisecond)
	for done := false; !done; {
		select {
		case m := <-r.commands:
			if m["type"] == "response.create" {
				t.Fatal("ツールの直接呼び出しで応答が始まりました")
			}
		case <-deadline:
			done = true
		}
	}
	var call, result bool
	for _, record := range r.history.Snapshot() {
		if record.Source != "automation/job1" {
			continue
		}
		call = call || (record.Role == types.RoleToolCall && strings.Contains(record.Text, `set_wake_mode: {"required":true}`))
		result = result || record.Role == types.RoleToolResult
	}
	if !call || !result {
		t.Fatal(r.history.Snapshot())
	}
}
