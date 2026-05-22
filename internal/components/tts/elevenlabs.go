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

	"smart-speaker/internal/graph"
	types "smart-speaker/internal/types"
)

const (
	elevenlabsSampleRate     = 24000
	elevenlabsBytesPerSample = 2
	elevenlabsChannels       = 1
	ttsPlaybackPaddingSec    = 0.5
)

type Config struct {
	APIKey        string
	Voice         string
	Model         string
	VoiceSettings *VoiceSettings
}

type VoiceSettings struct {
	Stability       float64
	SimilarityBoost float64
	Style           float64
	Speed           float64
	UseSpeakerBoost *bool
}

func NewStage(cfg Config) (*graph.Stage, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("elevenlabs: API key is required")
	}
	if cfg.Voice == "" {
		return nil, fmt.Errorf("elevenlabs: voice id is required")
	}
	if cfg.Model == "" {
		cfg.Model = "eleven_v3"
	}
	t := &streamTTS{
		cfg:        cfg,
		upstream:   make(chan types.Event, graph.DefaultChannelBufferSize),
		downstream: make(chan types.Event, graph.DefaultChannelBufferSize),
		client:     &http.Client{},
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
	client     *http.Client
	once       sync.Once
}

func (t *streamTTS) run(ctx context.Context) {
	defer close(t.downstream)
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-t.upstream:
			if !ok {
				return
			}
			if evt.Kind != types.EventTimelineItem {
				continue
			}
			item, ok := evt.Payload.(types.TimelineItem)
			if !ok {
				continue
			}
			if item.Kind != types.TimelineKindSpeech {
				t.emit(ctx, evt)
				continue
			}
			if strings.TrimSpace(item.Text) == "" {
				continue
			}
			t.handleSpeech(ctx, item)
		}
	}
}

func (t *streamTTS) emit(ctx context.Context, evt types.Event) {
	select {
	case <-ctx.Done():
	case t.downstream <- evt:
	}
}

func (t *streamTTS) handleSpeech(ctx context.Context, item types.TimelineItem) {
	audio, duration, err := t.synthesize(ctx, item.Text)
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("elevenlabs: synthesize error: %v", err)
		}
		return
	}
	playable := types.PlayableSpeech{
		GenerationID:     item.GenerationID,
		SequenceID:       item.SequenceID,
		Text:             item.Text,
		Audio:            audio,
		DurationSeconds:  duration,
		OriginalTimeline: item,
	}
	select {
	case <-ctx.Done():
	case t.downstream <- types.Event{Kind: types.EventPlayableSpeech, Payload: playable}:
	}
}

func (t *streamTTS) synthesize(ctx context.Context, text string) (string, float64, error) {
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
		return "", 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("xi-api-key", t.cfg.APIKey)

	resp, err := t.client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(resp.Body)
		return "", 0, fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, err
	}
	duration := ttsDurationSeconds(int64(len(raw)))
	log.Printf("elevenlabs: tts duration=%.3fs bytes=%d", duration, len(raw))
	return base64.StdEncoding.EncodeToString(raw), duration, nil
}

func (t *streamTTS) close() error {
	t.once.Do(func() {
		close(t.upstream)
	})
	return nil
}

func (t *streamTTS) buildVoiceSettings() map[string]any {
	defaultVS := VoiceSettings{
		Stability:       1.0,
		SimilarityBoost: 0.8,
		Speed:           1.2,
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
	return float64(bytes)/denom + ttsPlaybackPaddingSec
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
