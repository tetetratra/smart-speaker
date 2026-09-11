package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
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

func TestRegisterMemoryAPICreatesMemory(t *testing.T) {
	writer := &fakeManualMemoryWriter{
		createRecord: memorystate.Record{
			ID:      "memory-1",
			Content: "ユーザーは朝にコーヒーを飲む",
			Tags:    []string{"coffee"},
		},
	}
	mux := http.NewServeMux()
	registerMemoryAPI(mux, nil, writer)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/memories", strings.NewReader(`{"content":"ユーザーは朝にコーヒーを飲む"}`))
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if writer.createContent != "ユーザーは朝にコーヒーを飲む" {
		t.Fatalf("create content = %q", writer.createContent)
	}
	var got memoryWriteResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Memory.ID != "memory-1" || got.Memory.Tags[0] != "coffee" {
		t.Fatalf("response = %#v", got)
	}
}

func TestRegisterMemoryAPIUpdatesMemory(t *testing.T) {
	writer := &fakeManualMemoryWriter{
		updateRecord: memorystate.Record{
			ID:      "memory-1",
			Content: "ユーザーは朝に紅茶を飲む",
			Tags:    []string{"tea"},
		},
	}
	mux := http.NewServeMux()
	registerMemoryAPI(mux, nil, writer)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/memories/memory-1", strings.NewReader(`{"content":"ユーザーは朝に紅茶を飲む"}`))
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if writer.updateID != "memory-1" || writer.updateContent != "ユーザーは朝に紅茶を飲む" {
		t.Fatalf("update = id:%q content:%q", writer.updateID, writer.updateContent)
	}
	var got memoryWriteResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Memory.ID != "memory-1" || got.Memory.Tags[0] != "tea" {
		t.Fatalf("response = %#v", got)
	}
}

func TestRegisterMemoryAPIDeletesMemory(t *testing.T) {
	writer := &fakeManualMemoryWriter{}
	mux := http.NewServeMux()
	registerMemoryAPI(mux, nil, writer)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/memories/memory-1", nil))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if writer.deleteID != "memory-1" {
		t.Fatalf("delete id = %q", writer.deleteID)
	}
}

func TestRegisterMemoryAPIRejectsUnsupportedMethods(t *testing.T) {
	store, err := memorystate.NewStore(filepath.Join(t.TempDir(), "memory.json"))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerMemoryAPI(mux, store, &fakeManualMemoryWriter{})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/api/memories", nil))

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

func TestRegisterMemoryAPIReportsWriteErrors(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
	}{
		{name: "empty content", err: memorystate.ErrEmptyContent, status: http.StatusBadRequest},
		{name: "not found", err: memorystate.ErrRecordNotFound, status: http.StatusNotFound},
		{name: "internal", err: errors.New("failed"), status: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writer := &fakeManualMemoryWriter{updateErr: tt.err}
			mux := http.NewServeMux()
			registerMemoryAPI(mux, nil, writer)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPut, "/api/memories/memory-1", strings.NewReader(`{"content":"有効な記憶"}`))
			mux.ServeHTTP(rec, req)
			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d body=%s", rec.Code, tt.status, rec.Body.String())
			}
		})
	}
}

func TestManualMemoryServiceCreatesWithRecalculatedTagsAndEmbedding(t *testing.T) {
	store, err := memorystate.NewStore(filepath.Join(t.TempDir(), "memory.json"))
	if err != nil {
		t.Fatal(err)
	}
	tagger := &fakeMemoryTagger{tags: []string{"coffee", "morning"}}
	embedder := &fakeMemoryEmbedder{embedding: []float64{0.1, 0.2}}
	service := &manualMemoryService{
		store:                  store,
		tagger:                 tagger,
		embedder:               embedder,
		duplicateMinSimilarity: 0.98,
	}

	record, result, err := service.Create(context.Background(), "ユーザーは朝にコーヒーを飲む")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !result.Created {
		t.Fatal("Created = false, want true")
	}
	if tagger.content != "ユーザーは朝にコーヒーを飲む" {
		t.Fatalf("tagger content = %q", tagger.content)
	}
	if embedder.text != "ユーザーは朝にコーヒーを飲む coffee morning" {
		t.Fatalf("embed text = %q", embedder.text)
	}
	if !reflect.DeepEqual(record.Embedding, []float64{0.1, 0.2}) {
		t.Fatalf("embedding = %#v", record.Embedding)
	}
}

func TestManualMemoryServiceUpdatesWithRecalculatedTagsAndEmbedding(t *testing.T) {
	store, err := memorystate.NewStore(filepath.Join(t.TempDir(), "memory.json"))
	if err != nil {
		t.Fatal(err)
	}
	record, _, err := store.Upsert(memorystate.UpsertInput{Content: "古い記憶"})
	if err != nil {
		t.Fatal(err)
	}
	tagger := &fakeMemoryTagger{tags: []string{"tea"}}
	embedder := &fakeMemoryEmbedder{embedding: []float64{0.3, 0.4}}
	service := &manualMemoryService{store: store, tagger: tagger, embedder: embedder}

	updated, err := service.Update(context.Background(), record.ID, "ユーザーは朝に紅茶を飲む")
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if tagger.content != "ユーザーは朝に紅茶を飲む" {
		t.Fatalf("tagger content = %q", tagger.content)
	}
	if embedder.text != "ユーザーは朝に紅茶を飲む tea" {
		t.Fatalf("embed text = %q", embedder.text)
	}
	if !reflect.DeepEqual(updated.Tags, []string{"tea"}) {
		t.Fatalf("tags = %#v", updated.Tags)
	}
	if !reflect.DeepEqual(updated.Embedding, []float64{0.3, 0.4}) {
		t.Fatalf("embedding = %#v", updated.Embedding)
	}
}

type fakeManualMemoryWriter struct {
	createContent string
	updateID      string
	updateContent string
	deleteID      string
	createRecord  memorystate.Record
	updateRecord  memorystate.Record
	createResult  memorystate.UpsertResult
	createErr     error
	updateErr     error
	deleteErr     error
}

func (f *fakeManualMemoryWriter) Create(ctx context.Context, content string) (memorystate.Record, memorystate.UpsertResult, error) {
	f.createContent = content
	return f.createRecord, f.createResult, f.createErr
}

func (f *fakeManualMemoryWriter) Update(ctx context.Context, id, content string) (memorystate.Record, error) {
	f.updateID = id
	f.updateContent = content
	return f.updateRecord, f.updateErr
}

func (f *fakeManualMemoryWriter) Delete(id string) error {
	f.deleteID = id
	return f.deleteErr
}

type fakeMemoryTagger struct {
	content string
	tags    []string
	err     error
}

func (f *fakeMemoryTagger) CreateTags(ctx context.Context, content string) ([]string, error) {
	f.content = content
	if f.err != nil {
		return nil, f.err
	}
	return append([]string(nil), f.tags...), nil
}

type fakeMemoryEmbedder struct {
	text      string
	embedding []float64
	err       error
}

func (f *fakeMemoryEmbedder) Embed(ctx context.Context, text string) ([]float64, error) {
	f.text = text
	if f.err != nil {
		return nil, f.err
	}
	return append([]float64(nil), f.embedding...), nil
}
