package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/pion/webrtc/v4"
	"github.com/tetetratra/smart-speaker/internal/app"
	"github.com/tetetratra/smart-speaker/internal/components/wschat"
	"github.com/tetetratra/smart-speaker/internal/graph"
	"github.com/tetetratra/smart-speaker/internal/localverify"
	memorystate "github.com/tetetratra/smart-speaker/internal/states/memory"
	opus "gopkg.in/hraban/opus.v2"
	"net/http"
	"net/http/httptest"
	"nhooyr.io/websocket"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRealtimeConversationLocal(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	backend, handler := localverify.New(ctx)
	fake := httptest.NewServer(handler)
	defer fake.Close()
	cfg := app.Config{APIKey: "local", ResponsesModel: "local", RealtimeEndpoint: strings.Replace(fake.URL, "http", "ws", 1) + "/realtime", TTSProvider: "voicevox", Voicevox: app.VoicevoxConfig{Endpoint: fake.URL}, Memory: app.MemoryConfig{Model: "local"}, ConversationHistoryPath: filepath.Join(t.TempDir(), "conversation-history.json")}
	memory, err := memorystate.NewStore(filepath.Join(t.TempDir(), "memory.json"))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	chat := wschat.NewStage(mux, wschat.Config{})
	stages, err := buildStages(cfg, chat, memory, runtimeServices{})
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
	// ブラウザと同じOpus 2チャンネルで接続し、実際のRTP音声を受け取る。
	peer, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	connected := make(chan struct{})
	var connectedOnce sync.Once
	peer.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateConnected {
			connectedOnce.Do(func() { close(connected) })
		}
	})
	track, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2}, "mic", "browser")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = peer.AddTrack(track); err != nil {
		t.Fatal(err)
	}
	packets := make(chan time.Time, 1000)
	audioErrors := make(chan error, 1)
	peer.OnTrack(func(remote *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		decoder, err := opus.NewDecoder(48000, 2)
		if err != nil {
			audioErrors <- err
			return
		}
		for {
			packet, _, err := remote.ReadRTP()
			if err != nil {
				return
			}
			if _, err = decoder.Decode(packet.Payload, make([]int16, 1920)); err != nil {
				audioErrors <- err
				return
			}
			packets <- time.Now()
		}
	})
	offer, err := peer.CreateOffer(nil)
	if err != nil {
		t.Fatal(err)
	}
	gathered := webrtc.GatheringCompletePromise(peer)
	if err = peer.SetLocalDescription(offer); err != nil {
		t.Fatal(err)
	}
	select {
	case <-gathered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	rawOffer, _ := json.Marshal(map[string]any{"type": "webrtc.offer", "sdp": peer.LocalDescription().SDP})
	if err = browser.Write(ctx, websocket.MessageText, rawOffer); err != nil {
		t.Fatal(err)
	}
	messages := make(chan map[string]any, 1000)
	readErrors := make(chan error, 1)
	go func() {
		var pendingICE []webrtc.ICECandidateInit
		for {
			_, raw, err := browser.Read(ctx)
			if err != nil {
				readErrors <- err
				return
			}
			var m map[string]any
			if err = json.Unmarshal(raw, &m); err != nil {
				readErrors <- err
				return
			}
			switch m["type"] {
			case "webrtc.answer":
				if err = peer.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: m["sdp"].(string)}); err != nil {
					readErrors <- err
					return
				}
				for _, ice := range pendingICE {
					if err = peer.AddICECandidate(ice); err != nil {
						readErrors <- err
						return
					}
				}
				pendingICE = nil
			case "webrtc.ice":
				raw, _ := json.Marshal(m["candidate"])
				var ice webrtc.ICECandidateInit
				if err = json.Unmarshal(raw, &ice); err != nil {
					readErrors <- err
					return
				}
				if peer.RemoteDescription() == nil {
					pendingICE = append(pendingICE, ice)
					continue
				}
				if err = peer.AddICECandidate(ice); err != nil {
					readErrors <- err
					return
				}
			case "audio_frame":
				readErrors <- fmt.Errorf("WebSocketで音声データが送られた")
				return
			default:
				messages <- m
			}
		}
	}()
	read := func() map[string]any {
		t.Helper()
		select {
		case m := <-messages:
			return m
		case err := <-readErrors:
			t.Fatal(err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		return nil
	}
	for {
		m := read()
		if m["type"] == "realtime_status" && m["status"] == "connected" {
			break
		}
	}
	select {
	case <-connected:
	case err := <-readErrors:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal("WebRTC接続が成立しなかった")
	}
	if err = backend.Say("最初の会話"); err != nil {
		t.Fatal(err)
	}
	var firstGeneration float64
	for {
		m := read()
		if m["type"] == "playback_progress" && m["text"] != "" {
			firstGeneration = m["generation_id"].(float64)
			break
		}
	}
	select {
	case <-packets:
	case err := <-audioErrors:
		t.Fatal(err)
	case err := <-readErrors:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal("WebRTC音声が届かなかった")
	}
	var first time.Time
	select {
	case first = <-packets:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	last := first
	for i := 0; i < 9; i++ {
		select {
		case last = <-packets:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if last.Sub(first) < 100*time.Millisecond {
		t.Fatalf("音声が一括送信された: %v", last.Sub(first))
	}
	t.Logf("WebRTCのOpus音声を復号し、10パケットの到着間隔 %v を確認", last.Sub(first))
	t.Logf("最初の発話→文章分割→VOICEVOX互換API→世代%vの音声送信を確認", firstGeneration)
	if err = backend.Say("割り込み後の会話"); err != nil {
		t.Fatal(err)
	}
	stopped := false
	for {
		m := read()
		if m["type"] == "playback_stop" && m["generation_id"] == firstGeneration+1 {
			stopped = true
		}
		if m["type"] == "playback_progress" {
			gen := m["generation_id"].(float64)
			if stopped && gen < firstGeneration+1 {
				t.Fatal("停止通知後に旧世代音声を送信した", m)
			}
			if gen == firstGeneration+1 && m["text"] != "" {
				break
			}
		}
	}
	if !stopped {
		t.Fatal("再生停止通知が届かなかった")
	}
	t.Logf("割り込み→世代更新・再生停止→履歴補正→世代%vの音声送信を確認", firstGeneration+1)
}
