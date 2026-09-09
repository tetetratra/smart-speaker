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
	t.Setenv("STT_PROVIDER", "")
	t.Setenv("OPENAI_STT_MODEL", "")
	t.Setenv("TTS_PROVIDER", "")
	t.Setenv("VOICEVOX_SPEAKER_ID", "")
	t.Setenv("VOICEVOX_SPEED_SCALE", "")

	cfg := LoadConfig("")
	if cfg.STTProvider != "google" {
		t.Fatalf("STTProvider = %q, want google", cfg.STTProvider)
	}
	if cfg.OpenAISTTModel != "gpt-realtime-whisper" {
		t.Fatalf("OpenAISTTModel = %q, want gpt-realtime-whisper", cfg.OpenAISTTModel)
	}
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

func TestLoadConfigReadsSTTProviderConfig(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "openai")
	t.Setenv("STT_PROVIDER", "openai")
	t.Setenv("OPENAI_STT_MODEL", "gpt-live-transcribe")

	cfg := LoadConfig("")
	if cfg.STTProvider != "openai" {
		t.Fatalf("STTProvider = %q, want openai", cfg.STTProvider)
	}
	if cfg.OpenAISTTModel != "gpt-live-transcribe" {
		t.Fatalf("OpenAISTTModel = %q, want gpt-live-transcribe", cfg.OpenAISTTModel)
	}
}

func TestLoadConfigReadsRTCIceAdvertiseIPs(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "openai")
	t.Setenv("RTC_ICE_ADVERTISE_IPS", " 192.168.0.100,100.64.0.1, ,192.168.0.101 ")

	cfg := LoadConfig("")
	want := []string{"192.168.0.100", "100.64.0.1", "192.168.0.101"}
	if !reflect.DeepEqual(cfg.RTCIceAdvertiseIPs, want) {
		t.Fatalf("RTCIceAdvertiseIPs = %#v, want %#v", cfg.RTCIceAdvertiseIPs, want)
	}
}

func TestLoadConfigReadsMemoryConfig(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "openai")
	t.Setenv("OPENAI_RESPONSES_MODEL", "gpt-response")
	t.Setenv("OPENAI_MEMORY_MODEL", "gpt-memory")
	t.Setenv("MEMORY_STORE_PATH", "/app/data/memories.json")
	t.Setenv("MEMORY_EMBEDDING_BASE_URL", "http://embedding.local:8080")
	t.Setenv("MEMORY_EMBEDDING_MODEL", "model-name")
	t.Setenv("MEMORY_SIMILARITY_THRESHOLD", "0.82")
	t.Setenv("MEMORY_MAX_CONTEXT_MEMORIES", "4")
	t.Setenv("MEMORY_MAX_TAGS", "6")

	cfg := LoadConfig("")
	if cfg.Memory.Model != "gpt-memory" {
		t.Fatalf("Memory.Model = %q, want gpt-memory", cfg.Memory.Model)
	}
	if cfg.Memory.StorePath != "/app/data/memories.json" {
		t.Fatalf("Memory.StorePath = %q", cfg.Memory.StorePath)
	}
	if cfg.Memory.EmbeddingBaseURL != "http://embedding.local:8080" {
		t.Fatalf("Memory.EmbeddingBaseURL = %q", cfg.Memory.EmbeddingBaseURL)
	}
	if cfg.Memory.EmbeddingModel != "model-name" {
		t.Fatalf("Memory.EmbeddingModel = %q", cfg.Memory.EmbeddingModel)
	}
	if cfg.Memory.SimilarityThreshold != 0.82 {
		t.Fatalf("Memory.SimilarityThreshold = %f, want 0.82", cfg.Memory.SimilarityThreshold)
	}
	if cfg.Memory.MaxContextMemories != 4 {
		t.Fatalf("Memory.MaxContextMemories = %d, want 4", cfg.Memory.MaxContextMemories)
	}
	if cfg.Memory.MaxTags != 6 {
		t.Fatalf("Memory.MaxTags = %d, want 6", cfg.Memory.MaxTags)
	}
}

func TestLoadConfigDefaultsMemoryConfig(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "openai")
	t.Setenv("OPENAI_RESPONSES_MODEL", "gpt-response")
	t.Setenv("OPENAI_MEMORY_MODEL", "")
	t.Setenv("MEMORY_STORE_PATH", "")
	t.Setenv("MEMORY_EMBEDDING_BASE_URL", "")
	t.Setenv("MEMORY_EMBEDDING_MODEL", "")
	t.Setenv("MEMORY_SIMILARITY_THRESHOLD", "invalid")
	t.Setenv("MEMORY_MAX_CONTEXT_MEMORIES", "invalid")
	t.Setenv("MEMORY_MAX_TAGS", "invalid")

	cfg := LoadConfig("")
	if cfg.Memory.Model != "gpt-response" {
		t.Fatalf("Memory.Model = %q, want OPENAI_RESPONSES_MODEL fallback", cfg.Memory.Model)
	}
	if cfg.Memory.StorePath != "data/memories.json" {
		t.Fatalf("Memory.StorePath = %q, want data/memories.json", cfg.Memory.StorePath)
	}
	if cfg.Memory.EmbeddingBaseURL != "http://embedding:80" {
		t.Fatalf("Memory.EmbeddingBaseURL = %q, want http://embedding:80", cfg.Memory.EmbeddingBaseURL)
	}
	if cfg.Memory.EmbeddingModel != "intfloat/multilingual-e5-small" {
		t.Fatalf("Memory.EmbeddingModel = %q", cfg.Memory.EmbeddingModel)
	}
	if cfg.Memory.SimilarityThreshold != 0.7 {
		t.Fatalf("Memory.SimilarityThreshold = %f, want 0.7", cfg.Memory.SimilarityThreshold)
	}
	if cfg.Memory.MaxContextMemories != 3 {
		t.Fatalf("Memory.MaxContextMemories = %d, want 3", cfg.Memory.MaxContextMemories)
	}
	if cfg.Memory.MaxTags != 5 {
		t.Fatalf("Memory.MaxTags = %d, want 5", cfg.Memory.MaxTags)
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
