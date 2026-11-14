package wschat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tetetratra/smart-speaker/internal/states/playbackvolume"
	"nhooyr.io/websocket"
)

// 画面からの変更もツールからの変更も、接続中の全画面へ届き、後から開いた画面は現在の音量から始まる。
func TestPlaybackVolumeSync(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	store := &playbackvolume.Store{}
	mux := http.NewServeMux()
	RegisterPlaybackVolume(mux, store)
	server := httptest.NewServer(mux)
	defer server.Close()
	connect := func() *websocket.Conn {
		t.Helper()
		conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws/playback-volume", nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { conn.CloseNow() })
		return conn
	}
	read := func(conn *websocket.Conn) string {
		t.Helper()
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var got struct{ Type, Level string }
		if err := json.Unmarshal(data, &got); err != nil || got.Type != "playback_volume" {
			t.Fatalf("音量の通知ではない: %s", data)
		}
		return got.Level
	}

	first, second := connect(), connect()
	for _, conn := range []*websocket.Conn{first, second} {
		if got := read(conn); got != "boost" {
			t.Fatalf("初期値 = %s", got)
		}
	}
	if err := first.Write(ctx, websocket.MessageText, []byte(`{"type":"set_playback_volume","level":"low"}`)); err != nil {
		t.Fatal(err)
	}
	for _, conn := range []*websocket.Conn{first, second} {
		if got := read(conn); got != "low" {
			t.Fatalf("画面からの変更が届かない: %s", got)
		}
	}
	// 不正な値は無視し、続くツールからの変更だけが届く。
	if err := second.Write(ctx, websocket.MessageText, []byte(`{"type":"set_playback_volume","level":"loud"}`)); err != nil {
		t.Fatal(err)
	}
	store.Step(1)
	for _, conn := range []*websocket.Conn{first, second} {
		if got := read(conn); got != "normal" {
			t.Fatalf("ツールからの変更が届かない: %s", got)
		}
	}
	if got := read(connect()); got != "normal" {
		t.Fatalf("後から開いた画面の初期値 = %s", got)
	}
}
