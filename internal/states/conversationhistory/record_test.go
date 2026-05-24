package conversationhistory

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	types "smart-speaker/internal/types"
)

func TestToChatMessagesFormatsNaturalLanguageHistory(t *testing.T) {
	messages := ToChatMessages([]types.ConversationRecord{
		{Role: types.RoleUser, Text: "今日の天気どう？"},
		{Role: types.RoleAssistant, Text: "うん、見てみるね"},
		{
			Role:         types.RoleToolCall,
			Text:         `{"query":"東京 天気"}`,
			GenerationID: 3,
			Source:       "web_search",
			Metadata: map[string]any{
				"tool_call_id": "call-1",
				"tool_name":    "web_search",
			},
		},
		{
			Role:         types.RoleTool,
			Text:         `{"result":"晴れ"}`,
			GenerationID: 3,
			Source:       "web_search",
			Metadata: map[string]any{
				"tool_call_id": "call-1",
				"tool_name":    "weather_search",
			},
		},
	})

	if len(messages) != 4 {
		t.Fatalf("messages len = %d, want 4", len(messages))
	}
	if got, want := messages[0], (types.ChatMessage{Role: types.RoleUser, Content: "ユーザー: 今日の天気どう？"}); got != want {
		t.Fatalf("messages[0] = %#v, want %#v", got, want)
	}
	if got, want := messages[1], (types.ChatMessage{Role: types.RoleAssistant, Content: "あなた: うん、見てみるね"}); got != want {
		t.Fatalf("messages[1] = %#v, want %#v", got, want)
	}
	if messages[2].Role != types.RoleAssistant {
		t.Fatalf("messages[2].Role = %q, want assistant", messages[2].Role)
	}
	callPayload := parseToolCallContent(t, messages[2].Content)
	if got, want := callPayload["type"], "tool_call"; got != want {
		t.Fatalf("type = %v, want %v", got, want)
	}
	if got, want := callPayload["tool_name"], "web_search"; got != want {
		t.Fatalf("tool_name = %v, want %v", got, want)
	}
	if got, want := callPayload["tool_call_id"], "call-1"; got != want {
		t.Fatalf("tool_call_id = %v, want %v", got, want)
	}
	if got, want := callPayload["args"], map[string]any{"query": "東京 天気"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %#v, want %#v", got, want)
	}

	if messages[3].Role != types.RoleUser {
		t.Fatalf("messages[3].Role = %q, want user", messages[3].Role)
	}

	payload := parseToolContent(t, messages[3].Content)
	if got, want := payload["type"], "tool_result"; got != want {
		t.Fatalf("type = %v, want %v", got, want)
	}
	if got, want := payload["tool_name"], "weather_search"; got != want {
		t.Fatalf("tool_name = %v, want %v", got, want)
	}
	if got, want := payload["tool_call_id"], "call-1"; got != want {
		t.Fatalf("tool_call_id = %v, want %v", got, want)
	}
	if got, want := payload["generation_id"], float64(3); got != want {
		t.Fatalf("generation_id = %v, want %v", got, want)
	}
	if got, want := payload["output"], map[string]any{"result": "晴れ"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("output = %#v, want %#v", got, want)
	}
}

func TestToChatMessagesUsesSourceForMissingToolNameAndStringOutputFallback(t *testing.T) {
	messages := ToChatMessages([]types.ConversationRecord{
		{
			Role:         types.RoleTool,
			Text:         `plain output`,
			GenerationID: 4,
			Source:       "get_temp",
			Metadata: map[string]any{
				"tool_name": "",
				"stale":     true,
			},
		},
	})

	if len(messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(messages))
	}
	payload := parseToolContent(t, messages[0].Content)
	if got, want := payload["tool_name"], "get_temp"; got != want {
		t.Fatalf("tool_name = %v, want %v", got, want)
	}
	if got, want := payload["output"], "plain output"; got != want {
		t.Fatalf("output = %v, want %v", got, want)
	}
	if got, want := payload["stale"], true; got != want {
		t.Fatalf("stale = %v, want %v", got, want)
	}
}

func TestToChatMessagesKeepsToolNameWhenMetadataCannotBeMarshaled(t *testing.T) {
	messages := ToChatMessages([]types.ConversationRecord{
		{
			Role:         types.RoleTool,
			Text:         `{"ok":true}`,
			GenerationID: 5,
			Source:       "get_status",
			Metadata: map[string]any{
				"bad": make(chan struct{}),
			},
		},
	})

	if len(messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(messages))
	}
	payload := parseToolContent(t, messages[0].Content)
	if got, want := payload["tool_name"], "get_status"; got != want {
		t.Fatalf("tool_name = %v, want %v", got, want)
	}
	if _, ok := payload["bad"]; ok {
		t.Fatalf("bad metadata should be omitted: %#v", payload)
	}
}

func TestToChatMessagesSkipsEmptyRecordsAndKeepsUnknownRole(t *testing.T) {
	messages := ToChatMessages([]types.ConversationRecord{
		{Role: "", Text: "missing role"},
		{Role: types.RoleUser, Text: "   "},
		{Role: "system", Text: "keep as-is"},
	})

	want := []types.ChatMessage{{Role: "system", Content: "keep as-is"}}
	if !reflect.DeepEqual(messages, want) {
		t.Fatalf("messages = %#v, want %#v", messages, want)
	}
}

func parseToolContent(t *testing.T, content string) map[string]any {
	t.Helper()
	raw, ok := strings.CutPrefix(content, "ツール結果: ")
	if !ok {
		t.Fatalf("content = %q, want tool result prefix", content)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("unmarshal tool content: %v", err)
	}
	return payload
}

func parseToolCallContent(t *testing.T, content string) map[string]any {
	t.Helper()
	raw, ok := strings.CutPrefix(content, "ツール呼び出し: ")
	if !ok {
		t.Fatalf("content = %q, want tool call prefix", content)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("unmarshal tool call content: %v", err)
	}
	return payload
}
