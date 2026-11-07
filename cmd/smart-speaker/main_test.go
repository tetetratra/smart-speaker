package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	memorystate "github.com/tetetratra/smart-speaker/internal/states/memory"
	"github.com/tetetratra/smart-speaker/internal/tools/functions/switchbot"
)

func TestRegisterMemoryAPIListsMemories(t *testing.T) {
	store, err := memorystate.NewStore(filepath.Join(t.TempDir(), "memory.json"))
	if err != nil {
		t.Fatal(err)
	}
	oldRecord, _, err := store.Upsert(memorystate.UpsertInput{
		Content: "朝はコーヒーを飲む",
	})
	if err != nil {
		t.Fatal(err)
	}
	newRecord, _, err := store.Upsert(memorystate.UpsertInput{
		Content: "辛い料理が苦手",
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

func TestRegisterMemoryAPICreatesMemory(t *testing.T) {
	writer := &fakeManualMemoryWriter{
		createRecord: memorystate.Record{
			ID:      "memory-1",
			Content: "ユーザーは朝にコーヒーを飲む",
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
	if got.Memory.ID != "memory-1" {
		t.Fatalf("response = %#v", got)
	}
}

func TestRegisterMemoryAPIUpdatesMemory(t *testing.T) {
	writer := &fakeManualMemoryWriter{
		updateRecord: memorystate.Record{
			ID:      "memory-1",
			Content: "ユーザーは朝に紅茶を飲む",
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
	if got.Memory.ID != "memory-1" {
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

func TestRegisterSwitchBotSceneAPIListsScenes(t *testing.T) {
	client := &fakeSwitchBotSceneClient{
		scenes: []switchbot.Scene{
			{SceneID: "scene-1", SceneName: "換気扇をつける"},
			{SceneID: "", SceneName: "IDなし"},
			{SceneID: "scene-2", SceneName: ""},
			{SceneID: "scene-3", SceneName: "照明を消す"},
		},
	}
	mux := http.NewServeMux()
	registerSwitchBotSceneAPI(mux, client)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/switchbot/scenes", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got switchBotSceneListResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got.Scenes) != 2 {
		t.Fatalf("scenes len = %d, want 2: %#v", len(got.Scenes), got.Scenes)
	}
	if got.Scenes[0] != (switchBotSceneListItem{ID: "scene-1", Name: "換気扇をつける"}) {
		t.Fatalf("scenes[0] = %#v", got.Scenes[0])
	}
	if got.Scenes[1] != (switchBotSceneListItem{ID: "scene-3", Name: "照明を消す"}) {
		t.Fatalf("scenes[1] = %#v", got.Scenes[1])
	}
}

func TestRegisterSwitchBotSceneAPIExecutesScene(t *testing.T) {
	client := &fakeSwitchBotSceneClient{
		executeResult: map[string]any{"scene_id": "scene-1"},
	}
	mux := http.NewServeMux()
	registerSwitchBotSceneAPI(mux, client)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/switchbot/scenes/scene-1/execute", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if client.executedSceneID != "scene-1" {
		t.Fatalf("executed scene id = %q", client.executedSceneID)
	}
}

func TestRegisterSwitchBotSceneAPIReportsUnavailableClient(t *testing.T) {
	mux := http.NewServeMux()
	registerSwitchBotSceneAPI(mux, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/switchbot/scenes", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
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

type fakeSwitchBotSceneClient struct {
	scenes          []switchbot.Scene
	listErr         error
	executedSceneID string
	executeResult   map[string]any
	executeErr      error
}

func (f *fakeSwitchBotSceneClient) ListScenes(ctx context.Context) ([]switchbot.Scene, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return append([]switchbot.Scene(nil), f.scenes...), nil
}

func (f *fakeSwitchBotSceneClient) ExecuteScene(ctx context.Context, sceneID string) (map[string]any, error) {
	f.executedSceneID = sceneID
	if f.executeErr != nil {
		return nil, f.executeErr
	}
	return f.executeResult, nil
}

func TestManualMemoryServiceCreatesAndUpdatesContent(t *testing.T) {
	store, err := memorystate.NewStore(filepath.Join(t.TempDir(), "memory.json"))
	if err != nil {
		t.Fatal(err)
	}
	service := &manualMemoryService{store: store}

	record, result, err := service.Create(context.Background(), "ユーザーは朝にコーヒーを飲む")
	if err != nil || !result.Created || record.Content != "ユーザーは朝にコーヒーを飲む" {
		t.Fatalf("Create() = %#v, %#v, %v", record, result, err)
	}
	updated, err := service.Update(context.Background(), record.ID, "ユーザーは朝に紅茶を飲む")
	if err != nil || updated.ID != record.ID || updated.Content != "ユーザーは朝に紅茶を飲む" {
		t.Fatalf("Update() = %#v, %v", updated, err)
	}
}
