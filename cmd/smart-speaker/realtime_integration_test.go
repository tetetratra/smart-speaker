package main

import (
	"context"
	"encoding/json"
	"github.com/tetetratra/smart-speaker/internal/app"
	"github.com/tetetratra/smart-speaker/internal/components/wschat"
	"github.com/tetetratra/smart-speaker/internal/graph"
	"github.com/tetetratra/smart-speaker/internal/localverify"
	memorystate "github.com/tetetratra/smart-speaker/internal/states/memory"
	"github.com/tetetratra/smart-speaker/internal/types"
	"net/http"
	"net/http/httptest"
	"nhooyr.io/websocket"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRealtimeConversationLocal(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	backend, handler := localverify.New(ctx)
	fake := httptest.NewServer(handler)
	defer fake.Close()
	cfg := app.Config{APIKey: "local", ResponsesModel: "local", RealtimeEndpoint: strings.Replace(fake.URL, "http", "ws", 1) + "/realtime", TTSProvider: "voicevox", Voicevox: app.VoicevoxConfig{Endpoint: fake.URL}, Memory: app.MemoryConfig{Model: "local", EmbeddingBaseURL: fake.URL}}
	memory, err := memorystate.NewStore(filepath.Join(t.TempDir(), "memory.json"))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	chat := wschat.NewStage(mux, wschat.Config{})
	stages, err := buildStages(cfg, chat, memory)
	if err != nil {
		t.Fatal(err)
	}
	g := graph.New()
	wireGraph(g, stages)
	done := make(chan error, 1)
	go func() { done <- g.Run(ctx) }()
	defer func() { cancel(); <-done; g.Close() }()
	server := httptest.NewServer(mux)
	defer server.Close()
	browser, _, err := websocket.Dial(ctx, strings.Replace(server.URL, "http", "ws", 1)+"/ws/chat", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Close(websocket.StatusNormalClosure, "")
	read := func() map[string]any {
		t.Helper()
		_, raw, err := browser.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err = json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	stages.realtime.Upstream <- types.Event{Payload: types.RTCPeerOutputSink{PeerID: "local", Connected: true}}
	for {
		m := read()
		if m["type"] == "realtime_status" && m["status"] == "connected" {
			break
		}
	}
	if err = backend.Say("最初の会話"); err != nil {
		t.Fatal(err)
	}
	for {
		m := read()
		if m["type"] == "audio_frame" && m["audio"] != "" {
			if m["generation_id"] != float64(1) {
				t.Fatal(m)
			}
			break
		}
	}
	t.Log("最初の発話→文章分割→VOICEVOX互換API→世代1の音声送信を確認")
	if err = backend.Say("割り込み後の会話"); err != nil {
		t.Fatal(err)
	}
	stopped := false
	for {
		m := read()
		if m["type"] == "playback_stop" && m["generation_id"] == float64(2) {
			stopped = true
		}
		if m["type"] == "audio_frame" {
			gen := m["generation_id"].(float64)
			if stopped && gen < 2 {
				t.Fatal("停止通知後に旧世代音声を送信した", m)
			}
			if gen == 2 && m["audio"] != "" {
				break
			}
		}
	}
	if !stopped {
		t.Fatal("再生停止通知が届かなかった")
	}
	t.Log("割り込み→世代更新・再生停止→履歴補正→世代2の音声送信を確認")
}
