package tts

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVoicevoxSynthesizerSynthesizeSpeech(t *testing.T) {
	pcm := []byte{1, 2, 3, 4}
	var synthesisBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/audio_query":
			if r.Method != http.MethodPost {
				t.Fatalf("audio_query method = %s", r.Method)
			}
			if r.URL.Query().Get("text") != "こんにちは" {
				t.Fatalf("text = %q", r.URL.Query().Get("text"))
			}
			if r.URL.Query().Get("speaker") != "3" {
				t.Fatalf("speaker = %q", r.URL.Query().Get("speaker"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"speedScale":1.0,"kana":"abc"}`))
		case "/synthesis":
			if r.Method != http.MethodPost {
				t.Fatalf("synthesis method = %s", r.Method)
			}
			if r.URL.Query().Get("speaker") != "3" {
				t.Fatalf("speaker = %q", r.URL.Query().Get("speaker"))
			}
			if err := json.NewDecoder(r.Body).Decode(&synthesisBody); err != nil {
				t.Fatal(err)
			}
			_, _ = w.Write(testWAV(t, wavOptions{PCM: pcm}))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	speedScale := 1.25
	synth, err := newVoicevoxSynthesizer(VoicevoxConfig{
		Endpoint:   server.URL,
		SpeakerID:  3,
		SpeedScale: &speedScale,
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := synth.SynthesizeSpeech(context.Background(), "こんにちは")
	if err != nil {
		t.Fatal(err)
	}
	if string(got.PCM) != string(pcm) {
		t.Fatalf("pcm = %v, want %v", got.PCM, pcm)
	}
	if synthesisBody["speedScale"] != speedScale {
		t.Fatalf("speedScale = %v, want %v", synthesisBody["speedScale"], speedScale)
	}
}

func TestVoicevoxSynthesizerPreservesAudioQueryWithoutSpeedScale(t *testing.T) {
	var synthesisBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/audio_query":
			_, _ = w.Write([]byte(`{"speedScale":0.9}`))
		case "/synthesis":
			if err := json.NewDecoder(r.Body).Decode(&synthesisBody); err != nil {
				t.Fatal(err)
			}
			_, _ = w.Write(testWAV(t, wavOptions{}))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	synth, err := newVoicevoxSynthesizer(VoicevoxConfig{Endpoint: server.URL, SpeakerID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := synth.SynthesizeSpeech(context.Background(), "こんにちは"); err != nil {
		t.Fatal(err)
	}
	if synthesisBody["speedScale"] != 0.9 {
		t.Fatalf("speedScale = %v, want 0.9", synthesisBody["speedScale"])
	}
}

func TestVoicevoxSynthesizerReturnsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer server.Close()

	synth, err := newVoicevoxSynthesizer(VoicevoxConfig{Endpoint: server.URL, SpeakerID: 1})
	if err != nil {
		t.Fatal(err)
	}
	_, err = synth.SynthesizeSpeech(context.Background(), "こんにちは")
	if err == nil || !strings.Contains(err.Error(), "voicevox audio_query") {
		t.Fatalf("error = %v, want audio_query error", err)
	}
}

func TestVoicevoxSynthesizerReturnsWAVValidationError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/audio_query":
			_, _ = w.Write([]byte(`{"speedScale":1.0}`))
		case "/synthesis":
			_, _ = w.Write(testWAV(t, wavOptions{SampleRate: 48000}))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	synth, err := newVoicevoxSynthesizer(VoicevoxConfig{Endpoint: server.URL, SpeakerID: 1})
	if err != nil {
		t.Fatal(err)
	}
	_, err = synth.SynthesizeSpeech(context.Background(), "こんにちは")
	if err == nil || !strings.Contains(err.Error(), "sample rate") {
		t.Fatalf("error = %v, want sample rate error", err)
	}
}
