package tts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"smart-speaker/internal/graph"
	types "smart-speaker/internal/types"
)

// Config defines settings for ElevenLabs TTS.
type Config struct {
	APIKey string
	Voice  string
	Model  string
	// HTTPClient can be overridden for testing.
	HTTPClient *http.Client
}

// NewStage converts EventRealtimeOutput (assistant text) into EventRealtimeAudio using ElevenLabs TTS.
// モダリティが text のときにのみ利用する想定。
func NewStage(cfg Config) (*graph.Stage, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("elevenlabs: API key is required")
	}
	if cfg.Voice == "" {
		return nil, errors.New("elevenlabs: voice id is required")
	}
	if cfg.Model == "" {
		cfg.Model = "eleven_multilingual_v2"
	}
	t := &elevenLabsTTS{
		cfg:        cfg,
		httpClient: cfg.HTTPClient,
		upstream:   make(chan types.Event, graph.DefaultChannelBufferSize),
		downstream: make(chan types.Event, graph.DefaultChannelBufferSize),
	}
	if t.httpClient == nil {
		t.httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &graph.Stage{
		Upstream:   t.upstream,
		Downstream: t.downstream,
		Run:        t.run,
		CloseFn:    t.close,
	}, nil
}

type elevenLabsTTS struct {
	cfg        Config
	httpClient *http.Client
	upstream   chan types.Event
	downstream chan types.Event
}

func (t *elevenLabsTTS) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
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
			audio, err := t.synthesize(ctx, line.Text)
			if err != nil {
				log.Printf("elevenlabs tts error: %v", err)
				continue
			}
			select {
			case t.downstream <- types.Event{Kind: types.EventRealtimeAudio, Payload: types.OutputAudio{Role: "assistant", Audio: audio}}:
			case <-ctx.Done():
				return
			}
		}
	}
}

func (t *elevenLabsTTS) synthesize(ctx context.Context, text string) (string, error) {
	if text == "" {
		return "", errors.New("empty text")
	}
	url := fmt.Sprintf("https://api.elevenlabs.io/v1/text-to-speech/%s/stream?output_format=pcm_24000", t.cfg.Voice)
	body := map[string]any{
		"text":     text,
		"model_id": t.cfg.Model,
	}
	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("xi-api-key", t.cfg.APIKey)

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("elevenlabs: status %d: %s", resp.StatusCode, string(b))
	}
	audio, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(audio), nil
}

func (t *elevenLabsTTS) close() error {
	close(t.upstream)
	close(t.downstream)
	return nil
}
