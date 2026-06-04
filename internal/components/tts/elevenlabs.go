package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type ElevenLabsConfig struct {
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

func newElevenLabsSynthesizerFromConfig(cfg ElevenLabsConfig) (*elevenLabsSynthesizer, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("tts: elevenlabs API key is required")
	}
	if cfg.Voice == "" {
		return nil, fmt.Errorf("tts: elevenlabs voice id is required")
	}
	if cfg.Model == "" {
		cfg.Model = "eleven_v3"
	}
	return newElevenLabsSynthesizer(cfg), nil
}

type elevenLabsSynthesizer struct {
	cfg    ElevenLabsConfig
	client *http.Client
}

func newElevenLabsSynthesizer(cfg ElevenLabsConfig) *elevenLabsSynthesizer {
	return &elevenLabsSynthesizer{
		cfg:    cfg,
		client: &http.Client{},
	}
}

func (s *elevenLabsSynthesizer) Name() string {
	return "elevenlabs"
}

func (s *elevenLabsSynthesizer) SynthesizeSpeech(ctx context.Context, text string) (synthesizedSpeech, error) {
	url := fmt.Sprintf(
		"https://api.elevenlabs.io/v1/text-to-speech/%s/stream?output_format=pcm_24000",
		s.cfg.Voice,
	)
	payload := map[string]any{
		"text":          text,
		"model_id":      s.cfg.Model,
		"language_code": "ja",
	}
	if vs := s.buildVoiceSettings(); vs != nil {
		payload["voice_settings"] = vs
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return synthesizedSpeech{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return synthesizedSpeech{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("xi-api-key", s.cfg.APIKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return synthesizedSpeech{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(resp.Body)
		return synthesizedSpeech{}, fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return synthesizedSpeech{}, err
	}
	return synthesizedSpeech{PCM: raw}, nil
}

func (s *elevenLabsSynthesizer) buildVoiceSettings() map[string]any {
	defaultVS := VoiceSettings{
		Stability:       1.0,
		SimilarityBoost: 0.8,
		Speed:           1.2,
		UseSpeakerBoost: ptrBool(true),
	}

	vs := defaultVS
	if s.cfg.VoiceSettings != nil {
		if s.cfg.VoiceSettings.Stability != 0 {
			vs.Stability = s.cfg.VoiceSettings.Stability
		}
		if s.cfg.VoiceSettings.SimilarityBoost != 0 {
			vs.SimilarityBoost = s.cfg.VoiceSettings.SimilarityBoost
		}
		if s.cfg.VoiceSettings.Style != 0 {
			vs.Style = s.cfg.VoiceSettings.Style
		}
		if s.cfg.VoiceSettings.Speed != 0 {
			vs.Speed = s.cfg.VoiceSettings.Speed
		}
		if s.cfg.VoiceSettings.UseSpeakerBoost != nil {
			vs.UseSpeakerBoost = s.cfg.VoiceSettings.UseSpeakerBoost
		}
	}

	settings := map[string]any{
		"stability":        normalizeStability(s.cfg.Model, vs.Stability),
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
