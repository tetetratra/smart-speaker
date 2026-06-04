package tts

import "context"

const (
	ttsPCMOutputSampleRate     = 24000
	ttsPCMOutputBytesPerSample = 2
	ttsPCMOutputChannels       = 1
	ttsPlaybackPaddingSec      = 0.5
)

type synthesizedSpeech struct {
	PCM []byte
}

type speechSynthesizer interface {
	Name() string
	SynthesizeSpeech(ctx context.Context, text string) (synthesizedSpeech, error)
}

func ttsDurationSeconds(bytes int64) float64 {
	if bytes <= 0 {
		return 0
	}
	denom := float64(ttsPCMOutputSampleRate * ttsPCMOutputBytesPerSample * ttsPCMOutputChannels)
	return float64(bytes)/denom + ttsPlaybackPaddingSec
}
