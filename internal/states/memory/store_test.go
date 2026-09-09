package memory

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
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

func TestStoreUpsertCreatesAndPersistsRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.json")
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	record, result, err := store.Upsert(UpsertInput{
		Content:   "  朝はコーヒーを飲む  ",
		Tags:      []string{" preference ", "Preference", "morning"},
		Embedding: []float64{1, 0},
	})
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}
	if !result.Created {
		t.Fatal("UpsertResult.Created = false, want true")
	}
	if record.ID == "" {
		t.Fatal("Record.ID is empty")
	}
	if record.Content != "朝はコーヒーを飲む" {
		t.Fatalf("Content = %q, want trimmed content", record.Content)
	}
	if got, want := record.Tags, []string{"morning", "preference"}; !sameStrings(got, want) {
		t.Fatalf("Tags = %#v, want %#v", got, want)
	}

	reloaded, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore(reload) error = %v", err)
	}
	snapshot := reloaded.Snapshot()
	if len(snapshot) != 1 {
		t.Fatalf("reloaded Snapshot len = %d, want 1", len(snapshot))
	}
	if snapshot[0].ID != record.ID || snapshot[0].Content != record.Content {
		t.Fatalf("reloaded record = %#v, want %#v", snapshot[0], record)
	}
}

func TestStoreUpsertRejectsEmptyContent(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "memory.json"))
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	_, _, err = store.Upsert(UpsertInput{Content: "   "})
	if !errors.Is(err, ErrEmptyContent) {
		t.Fatalf("Upsert() error = %v, want ErrEmptyContent", err)
	}
}

func TestStoreUpsertSkipsDuplicateRecord(t *testing.T) {
	tests := []struct {
		name       string
		input      UpsertInput
		wantReason string
	}{
		{
			name: "content",
			input: UpsertInput{
				Content:   "犬を飼っている",
				Tags:      []string{"profile", "dog"},
				Embedding: []float64{0.8, 0.2},
			},
			wantReason: "content",
		},
		{
			name: "tags",
			input: UpsertInput{
				Content:   "猫を飼っている",
				Tags:      []string{"pet", "profile"},
				Embedding: []float64{0.7, 0.3},
			},
			wantReason: "tags",
		},
		{
			name: "embedding",
			input: UpsertInput{
				Content:                "雨の日は頭痛になりやすい",
				Tags:                   []string{"weather"},
				Embedding:              []float64{0.99, 0.01},
				DuplicateMinSimilarity: 0.98,
			},
			wantReason: "embedding",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, err := NewStore(filepath.Join(t.TempDir(), "memory.json"))
			if err != nil {
				t.Fatalf("NewStore() error = %v", err)
			}
			created, _, err := store.Upsert(UpsertInput{
				Content:   "犬を飼っている",
				Tags:      []string{"profile", "pet"},
				Embedding: []float64{1, 0},
			})
			if err != nil {
				t.Fatalf("Upsert(create) error = %v", err)
			}

			// 重複時に永続化しようとすると失敗するパスへ差し替え、保存もスキップされることを確認する。
			store.path = t.TempDir()
			got, result, err := store.Upsert(tt.input)
			if err != nil {
				t.Fatalf("Upsert(duplicate) error = %v", err)
			}
			if result.Created {
				t.Fatal("UpsertResult.Created = true, want false")
			}
			if result.DuplicateReason != tt.wantReason {
				t.Fatalf("DuplicateReason = %q, want %q", result.DuplicateReason, tt.wantReason)
			}
			if tt.wantReason == "embedding" && result.Similarity < tt.input.DuplicateMinSimilarity {
				t.Fatalf("Similarity = %f, want >= %f", result.Similarity, tt.input.DuplicateMinSimilarity)
			}
			if !reflect.DeepEqual(got, created) {
				t.Fatalf("Upsert() record = %#v, want unchanged %#v", got, created)
			}
			if snapshot := store.Snapshot(); !reflect.DeepEqual(snapshot, []Record{created}) {
				t.Fatalf("Snapshot() = %#v, want unchanged %#v", snapshot, []Record{created})
			}
		})
	}
}

func TestStoreFindDuplicate(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "memory.json"))
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	created, _, err := store.Upsert(UpsertInput{
		Content:   "週末はジョギングする",
		Tags:      []string{"routine"},
		Embedding: []float64{1, 0},
	})
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	found, ok := store.FindDuplicate(DuplicateInput{
		Content:       "別内容",
		Embedding:     []float64{0.99, 0.01},
		MinSimilarity: 0.98,
	})
	if !ok {
		t.Fatal("FindDuplicate() ok = false, want true")
	}
	if found.ID != created.ID {
		t.Fatalf("FindDuplicate ID = %q, want %q", found.ID, created.ID)
	}
	found.Content = "changed"
	if store.Snapshot()[0].Content == "changed" {
		t.Fatal("FindDuplicate returned internal record")
	}
}

func TestStoreSnapshotReturnsDeepCopy(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "memory.json"))
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	if _, _, err := store.Upsert(UpsertInput{
		Content:   "昼食は軽めがよい",
		Tags:      []string{"food"},
		Embedding: []float64{0.2, 0.8},
	}); err != nil {
		t.Fatalf("Upsert() error = %v", err)
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
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	if _, _, err := store.Upsert(UpsertInput{Content: "消す記憶"}); err != nil {
		t.Fatalf("Upsert() error = %v", err)
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
