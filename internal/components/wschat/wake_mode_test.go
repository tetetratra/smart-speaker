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

	"github.com/tetetratra/smart-speaker/internal/states/wakemode"
	"nhooyr.io/websocket"
)

func TestWakeModeSyncAndExpiry(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	settings := wakemode.DefaultSettings()
	settings.ConversationTimeout = 300 * time.Millisecond
	store, err := wakemode.NewStore(filepath.Join(t.TempDir(), "wake.json"), settings)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	RegisterWakeMode(mux, store)
	server := httptest.NewServer(mux)
	defer server.Close()
	connect := func() *websocket.Conn {
		t.Helper()
		conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws/wake-mode", nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { conn.CloseNow() })
		return conn
	}
	type state struct {
		Type     string `json:"type"`
		Required bool   `json:"required"`
		Awake    bool   `json:"awake"`
		Error    string `json:"error"`
	}
	read := func(conn *websocket.Conn) state {
		t.Helper()
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var got state
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	send := func(conn *websocket.Conn, body string) {
		t.Helper()
		if err := conn.Write(ctx, websocket.MessageText, []byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	first, second := connect(), connect()
	for _, conn := range []*websocket.Conn{first, second} {
		if got := read(conn); got.Type != "wake_mode" || got.Required || got.Awake {
			t.Fatalf("初期状態が違います: %+v", got)
		}
	}
	send(first, `{"type":"set_wake_mode","required":true}`)
	for {
		got := read(first)
		if !got.Required || got.Awake || got.Error != "" {
			t.Fatalf("設定変更が反映されていません: %+v", got)
		}
		if got.Type == "wake_mode_result" {
			break
		}
	}
	if got := read(second); !got.Required || got.Awake {
		t.Fatalf("他画面へ反映されていません: %+v", got)
	}
	// 外部APIと同じ経路で会話中にし、音声接続なしでも通知されることを確認する。
	store.Wake(time.Now())
	for _, conn := range []*websocket.Conn{first, second} {
		for {
			if got := read(conn); got.Awake {
				break
			}
		}
	}
	third := connect()
	if got := read(third); !got.Required || !got.Awake {
		t.Fatalf("再接続時の状態が違います: %+v", got)
	}
	store.Touch(time.Now())
	deadline := *store.Snapshot(time.Now()).AwakeUntil
	for _, conn := range []*websocket.Conn{first, second, third} {
		for {
			got := read(conn)
			if !got.Awake {
				if !got.Required || time.Now().Before(deadline) {
					t.Fatalf("期限より前に会話が終了しました: %+v", got)
				}
				break
			}
		}
	}
	send(first, `{"type":"set_wake_mode"}`)
	if got := read(first); got.Type != "wake_mode_result" || got.Error == "" || !got.Required {
		t.Fatalf("不正な操作の応答が違います: %+v", got)
	}
	// 音声判定で会話が始まる場合も同じ通知経路を通る。
	snapshot := store.Snapshot(time.Now())
	store.Evaluate("起きて", 1, snapshot.Revision, false, time.Now())
	if got := read(second); !got.Awake {
		t.Fatalf("音声判定が通知されていません: %+v", got)
	}
}
