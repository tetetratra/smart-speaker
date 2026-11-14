package wschat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

func TestConversationRecordsOnlySendDiff(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	long := strings.Repeat("長い日本語", 3000)
	mux := http.NewServeMux()
	stage := NewStage(mux)
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
		// 接続登録が済んだことを、シグナリングの受信で確認する。
		if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"webrtc.offer","sdp":"test"}`)); err != nil {
			t.Fatal(err)
		}
		select {
		case <-stage.Downstream:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
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
	result := types.ConversationRecord{ID: "result", Role: types.RoleToolResult, ToolName: "web_search", CallID: "c1", Output: long}
	stage.Upstream <- types.EventConversationRecordChanged{ConversationRecord: result}
	for _, conn := range []*websocket.Conn{first, second} {
		records := read(conn)
		if len(records) != 1 || records[0].ID != "result" || records[0].CallID != "c1" || records[0].Output != long {
			t.Fatal("差分が届きません", records)
		}
	}
	// 後から開いた画面にも過去の記録を再送せず、次の差分だけを送る。
	third := connect()
	result.ID = "next-result"
	stage.Upstream <- types.EventConversationRecordChanged{ConversationRecord: result}
	for _, conn := range []*websocket.Conn{first, second, third} {
		records := read(conn)
		if len(records) != 1 || records[0].ID != "next-result" || records[0].Output != long {
			t.Fatal("接続後の差分以外が届きました", records)
		}
	}
}
