package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type VoicevoxConfig struct {
	Endpoint   string
	SpeakerID  int
	SpeedScale *float64
}

type voicevoxSynthesizer struct {
	cfg    VoicevoxConfig
	client *http.Client
}

func newVoicevoxSynthesizer(cfg VoicevoxConfig) (*voicevoxSynthesizer, error) {
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return nil, fmt.Errorf("voicevox: endpoint is required")
	}
	if cfg.SpeakerID < 0 {
		return nil, fmt.Errorf("voicevox: speaker id must be non-negative")
	}
	if cfg.SpeedScale != nil && *cfg.SpeedScale <= 0 {
		return nil, fmt.Errorf("voicevox: speed scale must be positive")
	}
	return &voicevoxSynthesizer{
		cfg: VoicevoxConfig{
			Endpoint:   strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/"),
			SpeakerID:  cfg.SpeakerID,
			SpeedScale: cfg.SpeedScale,
		},
		client: &http.Client{},
	}, nil
}

func (s *voicevoxSynthesizer) Name() string {
	return "voicevox"
}

func (s *voicevoxSynthesizer) SynthesizeSpeech(ctx context.Context, text string) (synthesizedSpeech, error) {
	query, err := s.audioQuery(ctx, text)
	if err != nil {
		return synthesizedSpeech{}, err
	}
	wav, err := s.synthesis(ctx, query)
	if err != nil {
		return synthesizedSpeech{}, err
	}
	pcm, err := extractPCMFromWAV(wav)
	if err != nil {
		return synthesizedSpeech{}, err
	}
	return synthesizedSpeech{PCM: pcm}, nil
}

func (s *voicevoxSynthesizer) audioQuery(ctx context.Context, text string) ([]byte, error) {
	endpoint, err := s.apiURL("/audio_query", map[string]string{
		"text":    text,
		"speaker": strconv.Itoa(s.cfg.SpeakerID),
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, http.NoBody)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := readHTTPResponse(resp, "voicevox audio_query")
	if err != nil {
		return nil, err
	}
	if s.cfg.SpeedScale == nil {
		return body, nil
	}
	var query map[string]any
	if err := json.Unmarshal(body, &query); err != nil {
		return nil, fmt.Errorf("voicevox audio_query: decode response: %w", err)
	}
	query["speedScale"] = *s.cfg.SpeedScale
	body, err = json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("voicevox audio_query: encode response: %w", err)
	}
	return body, nil
}

func (s *voicevoxSynthesizer) synthesis(ctx context.Context, query []byte) ([]byte, error) {
	endpoint, err := s.apiURL("/synthesis", map[string]string{
		"speaker": strconv.Itoa(s.cfg.SpeakerID),
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(query))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return readHTTPResponse(resp, "voicevox synthesis")
}

func (s *voicevoxSynthesizer) apiURL(path string, params map[string]string) (string, error) {
	u, err := url.Parse(s.cfg.Endpoint + path)
	if err != nil {
		return "", err
	}
	q := u.Query()
	for key, value := range params {
		q.Set(key, value)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func readHTTPResponse(resp *http.Response, label string) ([]byte, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s: %s: %s", label, resp.Status, strings.TrimSpace(string(body)))
	}
	return body, nil
}
