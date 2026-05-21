package tts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"smart-speaker/internal/graph"
	types "smart-speaker/internal/types"
)

const (
	elevenlabsSampleRate     = 24000
	elevenlabsBytesPerSample = 2
	elevenlabsChannels       = 1
)

// Config defines settings for ElevenLabs stream-input TTS.
type Config struct {
	APIKey        string
	Voice         string
	Model         string
	VoiceSettings *VoiceSettings
	// HTTP headers: xi-api-key is required
}

// VoiceSettings represents ElevenLabs voice_settings payload.
// ゼロ値の場合はデフォルト値を適用する。
type VoiceSettings struct {
	Stability       float64
	SimilarityBoost float64
	Style           float64
	Speed           float64
	UseSpeakerBoost *bool
}

// NewStage converts EventRealtimeOutput (assistant text) stream into EventRealtimeAudio
// by using ElevenLabs stream-input WebSocket API.
func NewStage(cfg Config) (*graph.Stage, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("elevenlabs: API key is required")
	}
	if cfg.Voice == "" {
		return nil, fmt.Errorf("elevenlabs: voice id is required")
	}
	if cfg.Model == "" {
		// cfg.Model = "eleven_multilingual_v2"
		// cfg.Model = "eleven_ttv_v3"
		cfg.Model = "eleven_v3"
	}
	t := &streamTTS{
		cfg:        cfg,
		upstream:   make(chan types.Event, graph.DefaultChannelBufferSize),
		downstream: make(chan types.Event, graph.DefaultChannelBufferSize),
	}
	return &graph.Stage{
		Upstream:   t.upstream,
		Downstream: t.downstream,
		Run:        t.run,
		CloseFn:    t.close,
	}, nil
}

type streamTTS struct {
	cfg        Config
	upstream   chan types.Event
	downstream chan types.Event

	mu             sync.Mutex
	client         *http.Client
	cancelStream   context.CancelFunc
	streamResponse string
}

func (t *streamTTS) run(ctx context.Context) {
	if t.client == nil {
		t.client = &http.Client{}
	}
	for {
		select {
		case <-ctx.Done():
			t.cancelActiveStream()
			return
		case evt, ok := <-t.upstream:
			if !ok {
				return
			}
			if evt.Kind != types.EventRealtimeOutput {
				continue
			}
			line, ok := evt.Payload.(types.OutputLine)
			if !ok {
				continue
			}
			if line.Role != "" && line.Role != "assistant" {
				continue
			}
			if line.Final {
				continue
			}
			if line.Text == "" {
				continue
			}
			t.startStream(ctx, line.ResponseID, line.Text)
		}
	}
}

func (t *streamTTS) startStream(parent context.Context, respID string, text string) {
	t.mu.Lock()
	if t.streamResponse == respID {
		t.mu.Unlock()
		return
	}
	t.mu.Unlock()
	t.cancelActiveStream()

	ctx, cancel := context.WithCancel(parent)
	t.mu.Lock()
	t.cancelStream = cancel
	t.streamResponse = respID
	t.mu.Unlock()

	go t.streamRequest(ctx, respID, text)
}

func (t *streamTTS) streamRequest(ctx context.Context, respID string, text string) {
	url := fmt.Sprintf(
		"https://api.elevenlabs.io/v1/text-to-speech/%s/stream?output_format=pcm_24000",
		t.cfg.Voice,
	)
	payload := map[string]any{
		"text":          text,
		"model_id":      t.cfg.Model,
		"language_code": "ja",
	}
	if vs := t.buildVoiceSettings(); vs != nil {
		payload["voice_settings"] = vs
	}
	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("elevenlabs: json marshal error: %v", err)
		t.finishStream(respID)
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		log.Printf("elevenlabs: request error: %v", err)
		t.finishStream(respID)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("xi-api-key", t.cfg.APIKey)

	resp, err := t.client.Do(req)
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("elevenlabs: request error: %v", err)
		}
		t.finishStream(respID)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(resp.Body)
		log.Printf("elevenlabs: http error: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
		t.finishStream(respID)
		return
	}

	t.readStream(ctx, resp.Body, respID)
	t.finishStream(respID)
}

func (t *streamTTS) readStream(ctx context.Context, body io.Reader, respID string) {
	var totalBytes int64
	var audioStartAt time.Time
	buf := make([]byte, 4096)
	for {
		if ctx.Err() != nil {
			return
		}
		n, err := body.Read(buf)
		if n > 0 {
			if audioStartAt.IsZero() {
				audioStartAt = time.Now()
			}
			totalBytes += int64(n)
			audioB64 := base64.StdEncoding.EncodeToString(buf[:n])
			select {
			case t.downstream <- types.Event{Kind: types.EventRealtimeAudio, Payload: types.OutputAudio{Role: "assistant", Audio: audioB64}}:
			case <-ctx.Done():
				return
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			if ctx.Err() == nil {
				log.Printf("elevenlabs: read error: %v", err)
			}
			return
		}
	}
	if strings.TrimSpace(respID) == "" {
		return
	}
	seconds := ttsDurationSeconds(totalBytes)
	log.Printf("elevenlabs: tts duration=%.3fs bytes=%d response_id=%s", seconds, totalBytes, respID)
	if audioStartAt.IsZero() {
		audioStartAt = time.Now()
	}
	select {
	case t.downstream <- types.Event{Kind: types.EventTTSEnd, Payload: types.TTSEvent{
		ResponseID:      respID,
		AudioStartAt:    audioStartAt,
		DurationSeconds: seconds,
	}}:
	case <-ctx.Done():
	}
}

func (t *streamTTS) cancelActiveStream() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cancelStream != nil {
		t.cancelStream()
		t.cancelStream = nil
	}
	t.streamResponse = ""
}

func (t *streamTTS) finishStream(respID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.streamResponse != respID {
		return
	}
	t.streamResponse = ""
	t.cancelStream = nil
}

func (t *streamTTS) close() error {
	t.cancelActiveStream()
	close(t.upstream)
	close(t.downstream)
	return nil
}

func (t *streamTTS) buildVoiceSettings() map[string]any {
	// デフォルト値（ハードコード）
	defaultVS := VoiceSettings{
		Stability:       1.0, // v3 は 0.5 or 1.0 のみ有効
		SimilarityBoost: 0.8,
		Speed:           1.2, // 0.7–1.2 の範囲で、1.0がデフォルト
		UseSpeakerBoost: ptrBool(true),
	}

	vs := defaultVS
	if t.cfg.VoiceSettings != nil {
		if t.cfg.VoiceSettings.Stability != 0 {
			vs.Stability = t.cfg.VoiceSettings.Stability
		}
		if t.cfg.VoiceSettings.SimilarityBoost != 0 {
			vs.SimilarityBoost = t.cfg.VoiceSettings.SimilarityBoost
		}
		if t.cfg.VoiceSettings.Style != 0 {
			vs.Style = t.cfg.VoiceSettings.Style
		}
		if t.cfg.VoiceSettings.Speed != 0 {
			vs.Speed = t.cfg.VoiceSettings.Speed
		}
		if t.cfg.VoiceSettings.UseSpeakerBoost != nil {
			vs.UseSpeakerBoost = t.cfg.VoiceSettings.UseSpeakerBoost
		}
	}

	settings := map[string]any{
		"stability":        normalizeStability(t.cfg.Model, vs.Stability),
		"similarity_boost": vs.SimilarityBoost,
	}
	if vs.Style != 0 {
		settings["style"] = vs.Style
	}
	if vs.Speed != 0 {
		settings["speed"] = vs.Speed
	}
	if vs.UseSpeakerBoost != nil {
		settings["use_speaker_boost"] = *vs.UseSpeakerBoost
	}

	return settings
}

func ttsDurationSeconds(bytes int64) float64 {
	if bytes <= 0 {
		return 0
	}
	denom := float64(elevenlabsSampleRate * elevenlabsBytesPerSample * elevenlabsChannels)
	return float64(bytes) / denom
}

func ptrBool(v bool) *bool {
	return &v
}

func normalizeStability(model string, value float64) float64 {
	if !strings.HasPrefix(model, "eleven_v3") {
		return value
	}
	switch value {
	case 0, 0.5, 1:
		return value
	}
	if value < 0.25 {
		return 0
	}
	if value < 0.75 {
		return 0.5
	}
	return 1
}
