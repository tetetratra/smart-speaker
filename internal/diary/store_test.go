package diary

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreContentMigratesLegacyEntries(t *testing.T) {
	tmpDir := t.TempDir()
	legacyDir := filepath.Join(tmpDir, "tmp", "diary")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "2026-03-20.md"), []byte("# 2026-03-20 10:00\n一件目"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "2026-03-21.md"), []byte("# 2026-03-21 11:00\n二件目"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	store := NewStore(Config{
		Path:      filepath.Join(tmpDir, "data", "diary.md"),
		LegacyDir: legacyDir,
	})

	got, err := store.Content()
	if err != nil {
		t.Fatalf("Content() error = %v", err)
	}
	want := "# 2026-03-20 10:00\n一件目\n\n# 2026-03-21 11:00\n二件目"
	if got != want {
		t.Fatalf("Content() = %q, want %q", got, want)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "data", "diary.md"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != want+"\n" {
		t.Fatalf("persisted content = %q, want %q", string(data), want+"\n")
	}
}

func TestStoreAppendEntry(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "data", "diary.md")
	store := NewStore(Config{Path: path})

	when1 := time.Date(2026, 3, 21, 10, 30, 0, 0, time.Local)
	if _, err := store.AppendEntry(when1, "一行目\n二行目\n"); err != nil {
		t.Fatalf("AppendEntry() first error = %v", err)
	}

	when2 := time.Date(2026, 3, 21, 11, 45, 0, 0, time.Local)
	if _, err := store.AppendEntry(when2, "\n三行目"); err != nil {
		t.Fatalf("AppendEntry() second error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	want := "" +
		"# 2026-03-21 10:30\n" +
		"一行目\n二行目\n" +
		"\n\n" +
		"# 2026-03-21 11:45\n" +
		"三行目\n"
	if string(data) != want {
		t.Fatalf("diary file = %q, want %q", string(data), want)
	}
}
