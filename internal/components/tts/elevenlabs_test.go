package tts

import "testing"

func TestTTSDurationSeconds(t *testing.T) {
	bytes := int64(elevenlabsSampleRate * elevenlabsBytesPerSample * elevenlabsChannels)
	if got := ttsDurationSeconds(bytes); got != 1 {
		t.Fatalf("ttsDurationSeconds() = %v, want 1", got)
	}
}

func TestNormalizeStabilityForV3(t *testing.T) {
	if got := normalizeStability("eleven_v3", 0.6); got != 0.5 {
		t.Fatalf("normalizeStability() = %v, want 0.5", got)
	}
}
