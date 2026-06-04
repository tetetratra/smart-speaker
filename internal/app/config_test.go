package app

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestReadSTTPhrasesLoadsMainAndLocalFiles(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "stt_phrases.txt")
	localPath := filepath.Join(dir, "stt_phrases.local.txt")

	if err := os.WriteFile(mainPath, []byte("スマートスピーカー\n\n# comment\nyour-username\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(localPath, []byte("your-username\nPhraseSet\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := readSTTPhrases(mainPath)
	want := []string{"スマートスピーカー", "your-username", "PhraseSet"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %#v, got %#v", want, got)
	}
}

func TestReadSTTPhrasesIgnoresMissingFiles(t *testing.T) {
	got := readSTTPhrases(filepath.Join(t.TempDir(), "stt_phrases.txt"))
	if len(got) != 0 {
		t.Fatalf("expected no phrases, got %#v", got)
	}
}

func TestConversationIdleTimeoutFromEnv(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want time.Duration
	}{
		{name: "empty uses default", raw: "", want: 5 * time.Minute},
		{name: "positive seconds", raw: "42", want: 42 * time.Second},
		{name: "zero disables", raw: "0", want: 0},
		{name: "invalid uses default", raw: "invalid", want: 5 * time.Minute},
		{name: "negative uses default", raw: "-1", want: 5 * time.Minute},
		{name: "trims spaces", raw: " 5 ", want: 5 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := conversationIdleTimeoutFromEnv(tt.raw); got != tt.want {
				t.Fatalf("conversationIdleTimeoutFromEnv(%q) = %s, want %s", tt.raw, got, tt.want)
			}
		})
	}
}

func TestLoadConfigReadsTTSProviderConfig(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "openai")
	t.Setenv("TTS_PROVIDER", "voicevox")
	t.Setenv("VOICEVOX_ENDPOINT", "http://voicevox:50021")
	t.Setenv("VOICEVOX_SPEAKER_ID", "3")
	t.Setenv("VOICEVOX_SPEED_SCALE", "1.25")
	t.Setenv("ELEVENLABS_API_KEY", "")
	t.Setenv("ELEVENLABS_VOICE_ID", "")

	cfg := LoadConfig("")
	if cfg.TTSProvider != "voicevox" {
		t.Fatalf("TTSProvider = %q, want voicevox", cfg.TTSProvider)
	}
	if cfg.Voicevox.Endpoint != "http://voicevox:50021" {
		t.Fatalf("Voicevox.Endpoint = %q", cfg.Voicevox.Endpoint)
	}
	if cfg.Voicevox.SpeakerID != 3 {
		t.Fatalf("Voicevox.SpeakerID = %d, want 3", cfg.Voicevox.SpeakerID)
	}
	if cfg.Voicevox.SpeedScale == nil || *cfg.Voicevox.SpeedScale != 1.25 {
		t.Fatalf("Voicevox.SpeedScale = %v, want 1.25", cfg.Voicevox.SpeedScale)
	}
}

func TestLoadConfigDefaultsTTSProviderAndVoicevoxSettings(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "openai")
	t.Setenv("TTS_PROVIDER", "")
	t.Setenv("VOICEVOX_SPEAKER_ID", "")
	t.Setenv("VOICEVOX_SPEED_SCALE", "")

	cfg := LoadConfig("")
	if cfg.TTSProvider != "elevenlabs" {
		t.Fatalf("TTSProvider = %q, want elevenlabs", cfg.TTSProvider)
	}
	if cfg.Voicevox.SpeakerID != 1 {
		t.Fatalf("Voicevox.SpeakerID = %d, want 1", cfg.Voicevox.SpeakerID)
	}
	if cfg.Voicevox.SpeedScale != nil {
		t.Fatalf("Voicevox.SpeedScale = %v, want nil", *cfg.Voicevox.SpeedScale)
	}
}

func TestLoadConfigHandlesInvalidVoicevoxNumbers(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "openai")
	t.Setenv("VOICEVOX_SPEAKER_ID", "invalid")
	t.Setenv("VOICEVOX_SPEED_SCALE", "-1")

	cfg := LoadConfig("")
	if cfg.Voicevox.SpeakerID != 1 {
		t.Fatalf("Voicevox.SpeakerID = %d, want default 1", cfg.Voicevox.SpeakerID)
	}
	if cfg.Voicevox.SpeedScale != nil {
		t.Fatalf("Voicevox.SpeedScale = %v, want nil", *cfg.Voicevox.SpeedScale)
	}
}
