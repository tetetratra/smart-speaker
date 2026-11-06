package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tetetratra/smart-speaker/internal/app"
	"github.com/tetetratra/smart-speaker/internal/components/tts"
)

func TestTTSAPIThroughHTTPServer(t *testing.T) {
	selector, err := tts.NewSelector(tts.Config{Voicevox: tts.VoicevoxConfig{Endpoint: "http://localhost:50021"}, ElevenLabs: tts.ElevenLabsConfig{APIKey: "test-secret", Voice: "test-voice"}})
	if err != nil {
		t.Fatal(err)
	}
	cfg := app.Config{APIKey: "test-key", WebDistDir: t.TempDir()}
	cfg.Memory.Model = "test-model"
	server, chat, err := buildHTTPServer(cfg, nil, runtimeServices{TTS: selector})
	if err != nil {
		t.Fatal(err)
	}
	defer closeStages(chat)
	defer closeHTTPServer(server)
	for _, tt := range []struct {
		method, body string
		status       int
	}{
		{http.MethodGet, "", http.StatusOK},
		{http.MethodPut, `{"provider":"elevenlabs"}`, http.StatusOK},
		{http.MethodPut, `{"provider":"unknown"}`, http.StatusBadRequest},
		{http.MethodPut, `{"provider":"voicevox"} {}`, http.StatusBadRequest},
		{http.MethodPut, `{"provider":"voicevox","extra":true}`, http.StatusBadRequest},
		{http.MethodPut, `null`, http.StatusBadRequest},
		{http.MethodPut, `{"provider":"` + strings.Repeat("a", 5000) + `"}`, http.StatusBadRequest},
		{http.MethodDelete, "", http.StatusMethodNotAllowed},
	} {
		rec := httptest.NewRecorder()
		server.Handler.ServeHTTP(rec, httptest.NewRequest(tt.method, "/api/tts", strings.NewReader(tt.body)))
		if rec.Code != tt.status {
			t.Fatalf("%sの応答 = %d, 期待値 = %d: %s", tt.method, rec.Code, tt.status, rec.Body.String())
		}
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("選択状態のキャッシュが無効ではありません")
		}
		if strings.Contains(rec.Body.String(), "test-secret") || strings.Contains(rec.Body.String(), "test-voice") {
			t.Fatal("資格情報が応答に含まれています")
		}
	}
	if selector.Snapshot().Provider != tts.ProviderElevenLabs {
		t.Fatal("HTTP APIと生成側の選択が共有されていません")
	}
	rec := httptest.NewRecorder()
	server.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/tts", nil))
	var selection tts.Selection
	if err := json.Unmarshal(rec.Body.Bytes(), &selection); err != nil {
		t.Fatal(err)
	}
	if selection.Provider != tts.ProviderElevenLabs {
		t.Fatal("再取得時に選択が保持されていません")
	}
}

func TestTTSAPIUnavailableAndUnconfigured(t *testing.T) {
	selector, err := tts.NewSelector(tts.Config{Voicevox: tts.VoicevoxConfig{Endpoint: "http://localhost:50021"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		selector *tts.Selector
		status   int
	}{
		{nil, http.StatusServiceUnavailable},
		{selector, http.StatusBadRequest},
	} {
		mux := http.NewServeMux()
		registerTTSAPI(mux, tt.selector)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/tts", strings.NewReader(`{"provider":"elevenlabs"}`)))
		if rec.Code != tt.status {
			t.Fatalf("status = %d, 期待値 = %d", rec.Code, tt.status)
		}
	}
}
