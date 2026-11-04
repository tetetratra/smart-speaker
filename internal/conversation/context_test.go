package conversation

import (
	"context"
	"encoding/json"
	"github.com/tetetratra/smart-speaker/internal/types"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRestoreIncludesExecutedAndUnknownToolsWithoutReplaying(t *testing.T) {
	text, err := (ContextBuilder{}).Build(context.Background(), []types.ConversationRecord{
		{Role: types.RoleUser, Text: "照明つけて"}, {Role: types.RoleToolCall, Source: "light", Text: "実行開始・結果不明"}, {Role: types.RoleToolResult, Source: "light", Text: `{"ok":true}`},
	})
	if err != nil || text == "" {
		t.Fatal(text, err)
	}
}

func TestLongHistorySummarizesOldPartAndRetainsRecentText(t *testing.T) {
	var received string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if json.NewDecoder(r.Body).Decode(&payload) != nil {
			t.Error("要約要求を読めない")
		}
		received = payload["input"].([]any)[1].(map[string]any)["content"].(string)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"output":[{"content":[{"type":"output_text","text":"古い会話の要約"}]}]}`))
	}))
	defer server.Close()
	b := ContextBuilder{APIKey: "local", Model: "local", Endpoint: server.URL, MaxChars: 40}
	text, err := b.Build(context.Background(), []types.ConversationRecord{{Role: types.RoleUser, Text: strings.Repeat("古い発話", 40) + "最後の依頼"}})
	if err != nil || !strings.Contains(text, "古い会話の要約") || !strings.Contains(text, "最後の依頼") || strings.Contains(received, "最後の依頼") {
		t.Fatal(text, err, received)
	}
}
