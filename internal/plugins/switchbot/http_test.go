package switchbot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegisterSwitchBotSceneAPIListsScenes(t *testing.T) {
	client := &fakeSwitchBotSceneClient{
		scenes: []Scene{
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

type fakeSwitchBotSceneClient struct {
	scenes          []Scene
	listErr         error
	executedSceneID string
	executeResult   map[string]any
	executeErr      error
}

func (f *fakeSwitchBotSceneClient) ListScenes(ctx context.Context) ([]Scene, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return append([]Scene(nil), f.scenes...), nil
}

func (f *fakeSwitchBotSceneClient) ExecuteScene(ctx context.Context, sceneID string) (map[string]any, error) {
	f.executedSceneID = sceneID
	if f.executeErr != nil {
		return nil, f.executeErr
	}
	return f.executeResult, nil
}
