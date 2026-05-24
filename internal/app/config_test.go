package app

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
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
