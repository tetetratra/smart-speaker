package tts

import (
	"testing"

	pbspeed "github.com/tetetratra/smart-speaker/internal/states/playbackspeed"
)

func TestTTSDurationSeconds(t *testing.T) {
	bytes := int64(elevenlabsSampleRate * elevenlabsBytesPerSample * elevenlabsChannels)
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

func TestBuildVoiceSettingsAppliesPlaybackSpeedStore(t *testing.T) {
	store := pbspeed.NewStore()
	store.SetSpeed(2)
	tts := &streamTTS{cfg: Config{SpeedStore: store}}

	settings := tts.buildVoiceSettings()
	if got := settings["speed"]; got != 2.4 {
		t.Fatalf("speed = %v, want 2.4", got)
	}
}

func TestBuildVoiceSettingsCombinesConfiguredSpeedWithPlaybackSpeed(t *testing.T) {
	store := pbspeed.NewStore()
	store.SetSpeed(1.5)
	tts := &streamTTS{cfg: Config{
		VoiceSettings: &VoiceSettings{Speed: 1.1},
		SpeedStore:    store,
	}}

	settings := tts.buildVoiceSettings()
	if got := settings["speed"]; got != 1.6500000000000001 {
		t.Fatalf("speed = %v, want 1.65", got)
	}
}
