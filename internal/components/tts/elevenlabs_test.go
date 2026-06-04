package tts

import "testing"

func TestTTSDurationSeconds(t *testing.T) {
	bytes := int64(ttsPCMOutputSampleRate * ttsPCMOutputBytesPerSample * ttsPCMOutputChannels)
	if got := ttsDurationSeconds(bytes); got != 1.5 {
		t.Fatalf("ttsDurationSeconds() = %v, want 1.5", got)
	}
}

func TestTTSDurationSecondsReturnsZeroForEmptyAudio(t *testing.T) {
	if got := ttsDurationSeconds(0); got != 0 {
		t.Fatalf("ttsDurationSeconds() = %v, want 0", got)
	}
}

func TestNormalizeStabilityForV3(t *testing.T) {
	if got := normalizeStability("eleven_v3", 0.6); got != 0.5 {
		t.Fatalf("normalizeStability() = %v, want 0.5", got)
	}
}
