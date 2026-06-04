package tts

import (
	"context"
	"fmt"
	"strings"

	"github.com/tetetratra/smart-speaker/internal/graph"
)

const (
	ProviderElevenLabs = "elevenlabs"
	ProviderVoicevox   = "voicevox"

	ttsPCMOutputSampleRate     = 24000
	ttsPCMOutputBytesPerSample = 2
	ttsPCMOutputChannels       = 1
	ttsPlaybackPaddingSec      = 0.5
)

type Config struct {
	Provider   string
	ElevenLabs ElevenLabsConfig
	Voicevox   VoicevoxConfig
}

type synthesizedSpeech struct {
	PCM []byte
}

type speechSynthesizer interface {
	Name() string
	SynthesizeSpeech(ctx context.Context, text string) (synthesizedSpeech, error)
}

func NewStage(cfg Config) (*graph.Stage, error) {
	synthesizer, err := newSynthesizer(cfg)
	if err != nil {
		return nil, err
	}
	return newStageWithSynthesizer(synthesizer)
}

func newSynthesizer(cfg Config) (speechSynthesizer, error) {
	provider := strings.TrimSpace(cfg.Provider)
	if provider == "" {
		provider = ProviderElevenLabs
	}
	switch provider {
	case ProviderElevenLabs:
		return newElevenLabsSynthesizerFromConfig(cfg.ElevenLabs)
	case ProviderVoicevox:
		return newVoicevoxSynthesizer(cfg.Voicevox)
	default:
		return nil, fmt.Errorf("tts: unknown provider %q", provider)
	}
}

func ttsDurationSeconds(bytes int64) float64 {
	if bytes <= 0 {
		return 0
	}
	denom := float64(ttsPCMOutputSampleRate * ttsPCMOutputBytesPerSample * ttsPCMOutputChannels)
	return float64(bytes)/denom + ttsPlaybackPaddingSec
}
