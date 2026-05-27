package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewStoreLoadsPersistedRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.json")
	createdAt := time.Unix(1700000000, 0).UTC()
	payload := filePayload{
		Version: currentVersion,
		Records: []Record{
			{
				ID:        "memory-1",
				Content:   "  猫が好き  ",
				Tags:      []string{" pet ", "Pet", "", "profile"},
				Embedding: []float64{1, 0},
				CreatedAt: createdAt,
				UpdatedAt: createdAt,
			},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	snapshot := store.Snapshot()
	if len(snapshot) != 1 {
		t.Fatalf("Snapshot len = %d, want 1", len(snapshot))
	}
	if snapshot[0].Content != "猫が好き" {
		t.Fatalf("Content = %q, want trimmed content", snapshot[0].Content)
	}
	if got, want := snapshot[0].Tags, []string{"pet", "profile"}; !sameStrings(got, want) {
		t.Fatalf("Tags = %#v, want %#v", got, want)
	}
}

func TestStoreSnapshotReturnsDeepCopy(t *testing.T) {
	createdAt := time.Unix(1700000000, 0).UTC()
	store := &Store{
		records: []Record{
			{
				ID:        "memory-1",
				Content:   "昼食は軽めがよい",
				Tags:      []string{"food"},
				Embedding: []float64{0.2, 0.8},
				CreatedAt: createdAt,
				UpdatedAt: createdAt,
			},
		},
	}

	snapshot := store.Snapshot()
	snapshot[0].Tags[0] = "changed"
	snapshot[0].Embedding[0] = 99

	again := store.Snapshot()
	if again[0].Tags[0] == "changed" {
		t.Fatal("Tags changed through snapshot")
	}
	if again[0].Embedding[0] == 99 {
		t.Fatal("Embedding changed through snapshot")
	}
}

func TestStoreResetPersistsEmptyRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.json")
	createdAt := time.Unix(1700000000, 0).UTC()
	store := &Store{
		path: path,
		records: []Record{
			{
				ID:        "memory-1",
				Content:   "消す記憶",
				CreatedAt: createdAt,
				UpdatedAt: createdAt,
			},
		},
	}

	if err := store.Reset(); err != nil {
		t.Fatalf("Reset() error = %v", err)
	}
	if got := len(store.Snapshot()); got != 0 {
		t.Fatalf("Snapshot len = %d, want 0", got)
	}
	reloaded, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore(reload) error = %v", err)
	}
	if got := len(reloaded.Snapshot()); got != 0 {
		t.Fatalf("reloaded Snapshot len = %d, want 0", got)
	}
}

func TestRecordSearchText(t *testing.T) {
	record := Record{
		Content: "旅行が好き",
		Tags:    []string{" preference ", "", "travel"},
	}
	if got, want := record.SearchText(), "旅行が好き preference travel"; got != want {
		t.Fatalf("SearchText() = %q, want %q", got, want)
	}
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
