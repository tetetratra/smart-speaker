package memory

import (
	"encoding/json"
	"errors"
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

func TestStoreUpsertUpdatesDuplicateRecord(t *testing.T) {
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

	updated, result, err := store.Upsert(UpsertInput{
		Content:   "犬を飼っている",
		Tags:      []string{"profile", "dog"},
		Embedding: []float64{0.8, 0.2},
	})
	if err != nil {
		t.Fatalf("Upsert(update) error = %v", err)
	}
	if result.Created {
		t.Fatal("UpsertResult.Created = true, want false")
	}
	if result.DuplicateReason != "content" {
		t.Fatalf("DuplicateReason = %q, want content", result.DuplicateReason)
	}
	if updated.ID != created.ID {
		t.Fatalf("updated ID = %q, want %q", updated.ID, created.ID)
	}
	if !updated.CreatedAt.Equal(created.CreatedAt) {
		t.Fatalf("CreatedAt changed: %v -> %v", created.CreatedAt, updated.CreatedAt)
	}
	if !updated.UpdatedAt.After(created.UpdatedAt) && !updated.UpdatedAt.Equal(created.UpdatedAt) {
		t.Fatalf("UpdatedAt = %v, want >= %v", updated.UpdatedAt, created.UpdatedAt)
	}

	byTags, result, err := store.Upsert(UpsertInput{
		Content:   "タグ一致で更新する",
		Tags:      []string{"dog", "profile"},
		Embedding: []float64{0.7, 0.3},
	})
	if err != nil {
		t.Fatalf("Upsert(tag duplicate) error = %v", err)
	}
	if result.DuplicateReason != "tags" {
		t.Fatalf("DuplicateReason = %q, want tags", result.DuplicateReason)
	}
	if byTags.ID != created.ID {
		t.Fatalf("tag duplicate ID = %q, want %q", byTags.ID, created.ID)
	}
	if len(store.Snapshot()) != 1 {
		t.Fatalf("Snapshot len = %d, want 1", len(store.Snapshot()))
	}
}

func TestStoreUpsertUpdatesSimilarRecord(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "memory.json"))
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	created, _, err := store.Upsert(UpsertInput{
		Content:   "低気圧の日は頭痛になりやすい",
		Tags:      []string{"health"},
		Embedding: []float64{1, 0},
	})
	if err != nil {
		t.Fatalf("Upsert(create) error = %v", err)
	}

	updated, result, err := store.Upsert(UpsertInput{
		Content:                "雨の日は頭痛になりやすい",
		Tags:                   []string{"weather"},
		Embedding:              []float64{0.99, 0.01},
		DuplicateMinSimilarity: 0.98,
	})
	if err != nil {
		t.Fatalf("Upsert(similar) error = %v", err)
	}
	if result.Created {
		t.Fatal("UpsertResult.Created = true, want false")
	}
	if result.DuplicateReason != "embedding" {
		t.Fatalf("DuplicateReason = %q, want embedding", result.DuplicateReason)
	}
	if result.Similarity < 0.98 {
		t.Fatalf("Similarity = %f, want >= 0.98", result.Similarity)
	}
	if updated.ID != created.ID {
		t.Fatalf("updated ID = %q, want %q", updated.ID, created.ID)
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

func TestStoreSearchFiltersSortsAndLimitsResults(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "memory.json"))
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	inputs := []UpsertInput{
		{Content: "近い記憶", Tags: []string{"a"}, Embedding: []float64{1, 0}},
		{Content: "少し近い記憶", Tags: []string{"b"}, Embedding: []float64{0.8, 0.2}},
		{Content: "遠い記憶", Tags: []string{"c"}, Embedding: []float64{0, 1}},
		{Content: "embeddingなし", Tags: []string{"d"}},
		{Content: "次元違い", Tags: []string{"e"}, Embedding: []float64{1, 0, 0}},
	}
	for _, input := range inputs {
		if _, _, err := store.Upsert(input); err != nil {
			t.Fatalf("Upsert(%q) error = %v", input.Content, err)
		}
	}

	results := store.Search([]float64{1, 0}, SearchOptions{MinSimilarity: 0.7, Limit: 2})
	if len(results) != 2 {
		t.Fatalf("Search len = %d, want 2", len(results))
	}
	if results[0].Record.Content != "近い記憶" {
		t.Fatalf("results[0].Content = %q, want 近い記憶", results[0].Record.Content)
	}
	if results[1].Record.Content != "少し近い記憶" {
		t.Fatalf("results[1].Content = %q, want 少し近い記憶", results[1].Record.Content)
	}
	if results[0].Similarity < results[1].Similarity {
		t.Fatalf("results not sorted desc: %f < %f", results[0].Similarity, results[1].Similarity)
	}

	results[0].Record.Embedding[0] = 99
	again := store.Search([]float64{1, 0}, SearchOptions{MinSimilarity: 0.7, Limit: 1})
	if again[0].Record.Embedding[0] == 99 {
		t.Fatal("Search returned internal record")
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
