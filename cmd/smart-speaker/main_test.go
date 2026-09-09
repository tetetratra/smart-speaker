package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tetetratra/smart-speaker/internal/app"
	memorystate "github.com/tetetratra/smart-speaker/internal/states/memory"
)

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
	registerMemoryAPI(mux, store)
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

func TestRegisterMemoryAPIRejectsNonGet(t *testing.T) {
	store, err := memorystate.NewStore(filepath.Join(t.TempDir(), "memory.json"))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerMemoryAPI(mux, store)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/memories", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestRegisterMemoryAPIReportsMissingStore(t *testing.T) {
	mux := http.NewServeMux()
	registerMemoryAPI(mux, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/memories", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}
