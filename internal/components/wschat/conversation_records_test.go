package wschat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tetetratra/smart-speaker/internal/states/conversationhistory"
	"github.com/tetetratra/smart-speaker/internal/types"
	"nhooyr.io/websocket"
)

type recordView struct {
	ID        string `json:"id"`
	Role      string `json:"role"`
	ToolName  string `json:"tool_name"`
	CallID    string `json:"call_id"`
	Arguments string `json:"arguments"`
	Output    string `json:"output"`
}

func TestConversationRecordsInitialAndDiff(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	history, err := conversationhistory.NewStore(filepath.Join(t.TempDir(), "history.json"))
	if err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("長い日本語", 3000)
	for _, record := range []types.ConversationRecord{
		{ID: "user", Role: types.RoleUser, Text: "調べて"},
		{ID: "call", Role: types.RoleToolCall, ToolName: "web_search", CallID: "c1", Arguments: long},
	} {
		if err := history.Upsert(record); err != nil {
			t.Fatal(err)
		}
	}
	mux := http.NewServeMux()
	stage := NewStage(mux, Config{History: history})
	stage.Run(ctx)
	defer stage.CloseFn()
	server := httptest.NewServer(mux)
	defer server.Close()
	connect := func() *websocket.Conn {
		t.Helper()
		conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws/chat", nil)
		if err != nil {
			t.Fatal(err)
		}
		conn.SetReadLimit(1024 * 1024)
		t.Cleanup(func() { conn.CloseNow() })
		return conn
	}
	read := func(conn *websocket.Conn) []recordView {
		t.Helper()
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var msg struct {
			Type    string       `json:"type"`
			Records []recordView `json:"records"`
		}
		if err := json.Unmarshal(data, &msg); err != nil {
			t.Fatal(err)
		}
		if msg.Type != "conversation_records" {
			t.Fatalf("記録の通知ではありません: %s", data)
		}
		return msg.Records
	}
	first, second := connect(), connect()
	for _, conn := range []*websocket.Conn{first, second} {
		// 開いた直後は、会話でない記録だけを全文で送る。
		records := read(conn)
		if len(records) != 1 || records[0].ID != "call" || records[0].Arguments != long {
			t.Fatal("開いた直後の記録が違います", records)
		}
	}
	result := types.ConversationRecord{ID: "result", Role: types.RoleToolResult, ToolName: "web_search", CallID: "c1", Output: long}
	stage.Upstream <- types.EventConversationRecordChanged{ConversationRecord: result}
	for _, conn := range []*websocket.Conn{first, second} {
		records := read(conn)
		if len(records) != 1 || records[0].ID != "result" || records[0].CallID != "c1" || records[0].Output != long {
			t.Fatal("差分が届きません", records)
		}
	}
}

func TestWhiteboardRecordsKeepNewestWithinLimit(t *testing.T) {
	var records []types.ConversationRecord
	for i := 0; i < initialRecordLimit+10; i++ {
		records = append(records, types.ConversationRecord{ID: string(rune('a' + i%26)), Role: types.RoleSystem}, types.ConversationRecord{Role: types.RoleUser})
	}
	records = append(records, types.ConversationRecord{ID: "newest", Role: types.RoleToolCall})
	selected := whiteboardRecords(records)
	if len(selected) != initialRecordLimit || selected[len(selected)-1]["id"] != "newest" {
		t.Fatal(len(selected), selected[len(selected)-1])
	}
}
