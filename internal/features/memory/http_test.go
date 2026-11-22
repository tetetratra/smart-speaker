package memory

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegisterHTTPListsMemories(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "memory.json"))
	if err != nil {
		t.Fatal(err)
	}
	oldRecord, _, err := store.Upsert(UpsertInput{
		Content: "朝はコーヒーを飲む",
	})
	if err != nil {
		t.Fatal(err)
	}
	newRecord, _, err := store.Upsert(UpsertInput{
		Content: "辛い料理が苦手",
	})
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	registerHTTP(mux, store, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/memories", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", got)
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
	if got.Memories[0].CreatedAt.IsZero() || got.Memories[0].UpdatedAt.IsZero() {
		t.Fatalf("timestamps must be set: %#v", got.Memories[0])
	}
}

func TestRegisterHTTPCreatesMemory(t *testing.T) {
	writer := &fakeManualMemoryWriter{
		createRecord: Record{
			ID:      "memory-1",
			Content: "ユーザーは朝にコーヒーを飲む",
		},
	}
	mux := http.NewServeMux()
	registerHTTP(mux, nil, writer)
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
	if got.Memory.ID != "memory-1" {
		t.Fatalf("response = %#v", got)
	}
}

func TestRegisterHTTPUpdatesMemory(t *testing.T) {
	writer := &fakeManualMemoryWriter{
		updateRecord: Record{
			ID:      "memory-1",
			Content: "ユーザーは朝に紅茶を飲む",
		},
	}
	mux := http.NewServeMux()
	registerHTTP(mux, nil, writer)
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
	if got.Memory.ID != "memory-1" {
		t.Fatalf("response = %#v", got)
	}
}

func TestRegisterHTTPDeletesMemory(t *testing.T) {
	writer := &fakeManualMemoryWriter{}
	mux := http.NewServeMux()
	registerHTTP(mux, nil, writer)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/memories/memory-1", nil))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if writer.deleteID != "memory-1" {
		t.Fatalf("delete id = %q", writer.deleteID)
	}
}

func TestRegisterHTTPRejectsUnsupportedMethods(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "memory.json"))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerHTTP(mux, store, &fakeManualMemoryWriter{})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/api/memories", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestRegisterHTTPReportsMissingStore(t *testing.T) {
	mux := http.NewServeMux()
	registerHTTP(mux, nil, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/memories", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestRegisterHTTPReportsWriteErrors(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
	}{
		{name: "empty content", err: ErrEmptyContent, status: http.StatusBadRequest},
		{name: "not found", err: ErrRecordNotFound, status: http.StatusNotFound},
		{name: "internal", err: errors.New("failed"), status: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writer := &fakeManualMemoryWriter{updateErr: tt.err}
			mux := http.NewServeMux()
			registerHTTP(mux, nil, writer)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPut, "/api/memories/memory-1", strings.NewReader(`{"content":"有効な記憶"}`))
			mux.ServeHTTP(rec, req)
			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d body=%s", rec.Code, tt.status, rec.Body.String())
			}
		})
	}
}

type fakeManualMemoryWriter struct {
	createContent string
	updateID      string
	updateContent string
	deleteID      string
	createRecord  Record
	updateRecord  Record
	createResult  UpsertResult
	createErr     error
	updateErr     error
	deleteErr     error
}

func (f *fakeManualMemoryWriter) Create(ctx context.Context, content string) (Record, UpsertResult, error) {
	f.createContent = content
	return f.createRecord, f.createResult, f.createErr
}

func (f *fakeManualMemoryWriter) Update(ctx context.Context, id, content string) (Record, error) {
	f.updateID = id
	f.updateContent = content
	return f.updateRecord, f.updateErr
}

func (f *fakeManualMemoryWriter) Delete(id string) error {
	f.deleteID = id
	return f.deleteErr
}
