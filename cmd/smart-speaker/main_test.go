package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tetetratra/smart-speaker/internal/app"
	memorystate "github.com/tetetratra/smart-speaker/internal/states/memory"
)

type fakeMemoryMetadataBuilder struct {
	tags      []string
	embedding []float64
	err       error
	calls     []string
}

func (f *fakeMemoryMetadataBuilder) Build(_ context.Context, content string) ([]string, []float64, error) {
	f.calls = append(f.calls, content)
	if f.err != nil {
		return nil, nil, f.err
	}
	return append([]string(nil), f.tags...), append([]float64(nil), f.embedding...), nil
}

func TestBuildSTTStageDefaultsToGoogle(t *testing.T) {
	st, err := buildSTTStage(app.Config{
		APIKey: "openai",
	})
	if err != nil {
		t.Fatal(err)
	}
	if st == nil {
		t.Fatal("expected stage")
	}
	_ = st.Close()
}

func TestBuildSTTStageSelectsOpenAI(t *testing.T) {
	st, err := buildSTTStage(app.Config{
		APIKey:         "openai",
		STTProvider:    "openai",
		OpenAISTTModel: "gpt-realtime-whisper",
	})
	if err != nil {
		t.Fatal(err)
	}
	if st == nil {
		t.Fatal("expected stage")
	}
	_ = st.Close()
}

func TestBuildSTTStageRejectsUnknownProvider(t *testing.T) {
	if _, err := buildSTTStage(app.Config{STTProvider: "unknown"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestRegisterMemoryAPIListsMemoriesWithoutEmbeddings(t *testing.T) {
	store, err := memorystate.NewStore(filepath.Join(t.TempDir(), "memory.json"))
	if err != nil {
		t.Fatal(err)
	}
	oldRecord, _, err := store.Upsert(memorystate.UpsertInput{
		Content:                "朝はコーヒーを飲む",
		Tags:                   []string{"coffee"},
		Embedding:              []float64{0.1, 0.2},
		DuplicateMinSimilarity: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	newRecord, _, err := store.Upsert(memorystate.UpsertInput{
		Content:                "辛い料理が苦手",
		Tags:                   []string{"food"},
		Embedding:              []float64{0.3, 0.4},
		DuplicateMinSimilarity: 2,
	})
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	registerMemoryAPI(mux, store, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/memories", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	if strings.Contains(rec.Body.String(), "embedding") {
		t.Fatalf("response contains embedding: %s", rec.Body.String())
	}
	var got memoryListResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got.Memories) != 2 {
		t.Fatalf("memories len = %d, want 2", len(got.Memories))
	}
	if got.Memories[0].ID != newRecord.ID || got.Memories[1].ID != oldRecord.ID {
		t.Fatalf("memory order = [%s, %s], want [%s, %s]", got.Memories[0].ID, got.Memories[1].ID, newRecord.ID, oldRecord.ID)
	}
	if got.Memories[0].Content != newRecord.Content {
		t.Fatalf("content = %q, want %q", got.Memories[0].Content, newRecord.Content)
	}
	if len(got.Memories[0].Tags) != 1 || got.Memories[0].Tags[0] != "food" {
		t.Fatalf("tags = %#v, want [food]", got.Memories[0].Tags)
	}
	if got.Memories[0].CreatedAt.IsZero() || got.Memories[0].UpdatedAt.IsZero() {
		t.Fatalf("timestamps must be set: %#v", got.Memories[0])
	}
}

func TestRegisterMemoryAPICreatesMemoryWithRecomputedMetadata(t *testing.T) {
	store, err := memorystate.NewStore(filepath.Join(t.TempDir(), "memory.json"))
	if err != nil {
		t.Fatal(err)
	}
	metadata := &fakeMemoryMetadataBuilder{
		tags:      []string{"drink", "morning"},
		embedding: []float64{0.3, 0.4},
	}

	mux := http.NewServeMux()
	registerMemoryAPI(mux, store, metadata)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/memories", bytes.NewBufferString(`{"content":"朝は水を飲む"}`)))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var got memoryItemResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Memory.Content != "朝は水を飲む" {
		t.Fatalf("content = %q, want created content", got.Memory.Content)
	}
	if len(got.Memory.Tags) != 2 || got.Memory.Tags[0] != "drink" || got.Memory.Tags[1] != "morning" {
		t.Fatalf("tags = %#v, want recomputed tags", got.Memory.Tags)
	}
	if len(metadata.calls) != 1 || metadata.calls[0] != "朝は水を飲む" {
		t.Fatalf("metadata calls = %#v, want input content", metadata.calls)
	}
	snapshot := store.Snapshot()
	if len(snapshot) != 1 {
		t.Fatalf("Snapshot len = %d, want 1", len(snapshot))
	}
	if !sameFloat64s(snapshot[0].Embedding, []float64{0.3, 0.4}) {
		t.Fatalf("embedding = %#v, want recomputed embedding", snapshot[0].Embedding)
	}
}

func TestRegisterMemoryAPIUpdatesMemoryWithRecomputedMetadata(t *testing.T) {
	store, err := memorystate.NewStore(filepath.Join(t.TempDir(), "memory.json"))
	if err != nil {
		t.Fatal(err)
	}
	record, _, err := store.Upsert(memorystate.UpsertInput{
		Content:   "朝はコーヒーを飲む",
		Tags:      []string{"coffee"},
		Embedding: []float64{0.1, 0.2},
	})
	if err != nil {
		t.Fatal(err)
	}
	metadata := &fakeMemoryMetadataBuilder{
		tags:      []string{"tea"},
		embedding: []float64{0.5, 0.6},
	}

	mux := http.NewServeMux()
	registerMemoryAPI(mux, store, metadata)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/api/memories/"+record.ID, bytes.NewBufferString(`{"content":"朝は紅茶を飲む"}`)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got memoryItemResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Memory.ID != record.ID || got.Memory.Content != "朝は紅茶を飲む" {
		t.Fatalf("memory = %#v, want updated record", got.Memory)
	}
	snapshot := store.Snapshot()
	if len(snapshot) != 1 {
		t.Fatalf("Snapshot len = %d, want 1", len(snapshot))
	}
	if !sameFloat64s(snapshot[0].Embedding, []float64{0.5, 0.6}) {
		t.Fatalf("embedding = %#v, want recomputed embedding", snapshot[0].Embedding)
	}
}

func TestRegisterMemoryAPIDeletesMemory(t *testing.T) {
	store, err := memorystate.NewStore(filepath.Join(t.TempDir(), "memory.json"))
	if err != nil {
		t.Fatal(err)
	}
	record, _, err := store.Upsert(memorystate.UpsertInput{
		Content: "朝はコーヒーを飲む",
	})
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	registerMemoryAPI(mux, store, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/memories/"+record.ID, nil))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if snapshot := store.Snapshot(); len(snapshot) != 0 {
		t.Fatalf("Snapshot len = %d, want 0", len(snapshot))
	}
}

func TestRegisterMemoryAPIRejectsUnsupportedCollectionMethod(t *testing.T) {
	store, err := memorystate.NewStore(filepath.Join(t.TempDir(), "memory.json"))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerMemoryAPI(mux, store, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/memories", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestRegisterMemoryAPIReportsMissingStore(t *testing.T) {
	mux := http.NewServeMux()
	registerMemoryAPI(mux, nil, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/memories", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func sameFloat64s(a, b []float64) bool {
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
