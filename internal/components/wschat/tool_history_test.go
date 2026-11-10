package wschat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tetetratra/smart-speaker/internal/states/toolhistory"
	"github.com/tetetratra/smart-speaker/internal/types"
	"nhooyr.io/websocket"
)

func TestToolHistoryPushAndReconnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	store := toolhistory.NewStore(2)
	mux := http.NewServeMux()
	stage := NewStage(mux, Config{ToolHistory: store})
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
	read := func(conn *websocket.Conn) []toolhistory.Entry {
		t.Helper()
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var msg struct {
			Type    string              `json:"type"`
			Entries []toolhistory.Entry `json:"entries"`
		}
		if err := json.Unmarshal(data, &msg); err != nil {
			t.Fatal(err)
		}
		if msg.Type != "tool_history" || msg.Entries == nil {
			t.Fatalf("履歴一覧ではない通知です: %s", data)
		}
		return msg.Entries
	}
	first, second := connect(), connect()
	if len(read(first)) != 0 || len(read(second)) != 0 {
		t.Fatal("初回の空一覧を取得できません")
	}
	long := strings.Repeat("長い日本語", 3000)
	args, _ := json.Marshal(map[string]string{"query": long})
	output, _ := json.Marshal(map[string]string{"result": long})
	id := store.Begin(types.ToolRequest{ToolCallID: "call-1", Name: "web_search", Arguments: args})
	for _, conn := range []*websocket.Conn{first, second} {
		entries := read(conn)
		if len(entries) != 1 || entries[0].Status != "running" || entries[0].Arguments != string(args) {
			t.Fatal("開始状態または引数が欠落しました")
		}
	}
	first.CloseNow()
	store.Finish(id, types.ToolResultRecord{Output: output})
	entries := read(second)
	if len(entries) != 1 || entries[0].Status != "succeeded" || entries[0].Output != string(output) {
		t.Fatal("終了状態または結果が欠落しました")
	}
	entries = read(connect())
	if len(entries) != 1 || entries[0].Arguments != string(args) || entries[0].Output != string(output) || entries[0].Status != "succeeded" {
		t.Fatal("再接続時に最新の一覧を復元できません")
	}
	message := types.InboundMessage{Source: "tool/web_search", Text: long + "\n末尾"}
	store.RecordInbound(message, 1)
	entries = read(second)
	if len(entries) != 2 || entries[0].Kind != "inbound" || entries[0].Source != message.Source || entries[0].Text != message.Text || entries[1].Kind != "tool" {
		t.Fatal("差し込みがツールと同じ一覧に届きません", entries)
	}
	entries = read(connect())
	if len(entries) != 2 || entries[0].Text != message.Text || entries[1].Output != string(output) {
		t.Fatal("再接続で差し込みかツールの全文が欠落しました")
	}

}
