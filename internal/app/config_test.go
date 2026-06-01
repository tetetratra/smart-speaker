package app

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	hookmemory "github.com/tetetratra/smart-speaker/internal/hooks/memory"
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

func TestMemoryEmbeddingConfigFromEnvUsesDefaults(t *testing.T) {
	t.Setenv("MEMORY_EMBEDDING_BASE_URL", "")
	t.Setenv("MEMORY_EMBEDDING_MODEL", "")
	t.Setenv("MEMORY_EMBEDDING_PROMPT_NAME", "")

	got := memoryEmbeddingConfigFromEnv()
	want := MemoryEmbeddingConfig{
		BaseURL: hookmemory.DefaultEmbeddingBaseURL,
		Model:   hookmemory.DefaultEmbeddingModel,
	}
	if got != want {
		t.Fatalf("memoryEmbeddingConfigFromEnv() = %#v, want %#v", got, want)
	}
}

func TestMemoryEmbeddingConfigFromEnvOverridesValues(t *testing.T) {
	t.Setenv("MEMORY_EMBEDDING_BASE_URL", " http://localhost:8080 ")
	t.Setenv("MEMORY_EMBEDDING_MODEL", " custom-model ")
	t.Setenv("MEMORY_EMBEDDING_PROMPT_NAME", " query ")

	got := memoryEmbeddingConfigFromEnv()
	want := MemoryEmbeddingConfig{
		BaseURL:    "http://localhost:8080",
		Model:      "custom-model",
		PromptName: "query",
	}
	if got != want {
		t.Fatalf("memoryEmbeddingConfigFromEnv() = %#v, want %#v", got, want)
	}
}
