package switchbot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestClient(server *httptest.Server) *Client {
	client := NewSwitchbotClient("token", "secret", `{"hub2":"hub-device"}`)
	client.baseURL = server.URL
	client.http = server.Client()
	return client
}

func TestClientListScenes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1.1/scenes" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") == "" || r.Header.Get("sign") == "" {
			t.Fatalf("missing auth headers: %#v", r.Header)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"statusCode": 100,
			"message":    "success",
			"body": []map[string]string{
				{"sceneId": "scene-1", "sceneName": "換気扇をつける"},
				{"sceneId": "", "sceneName": "IDなし"},
				{"sceneId": "scene-2", "sceneName": ""},
				{"sceneId": "scene-3", "sceneName": "照明を消す"},
			},
		})
	}))
	defer server.Close()

	scenes, err := newTestClient(server).ListScenes(context.Background())
	if err != nil {
		t.Fatalf("ListScenes() error = %v", err)
	}
	if len(scenes) != 2 {
		t.Fatalf("len(scenes) = %d, want 2: %#v", len(scenes), scenes)
	}
	if scenes[0] != (Scene{SceneID: "scene-1", SceneName: "換気扇をつける"}) {
		t.Fatalf("scenes[0] = %#v", scenes[0])
	}
	if scenes[1] != (Scene{SceneID: "scene-3", SceneName: "照明を消す"}) {
		t.Fatalf("scenes[1] = %#v", scenes[1])
	}
}

func TestClientListScenesDecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("{"))
	}))
	defer server.Close()

	if _, err := newTestClient(server).ListScenes(context.Background()); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestClientListScenesRejectsUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("switchbot-request-id", "req-scenes-401")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": "Unauthorized",
		})
	}))
	defer server.Close()

	_, err := newTestClient(server).ListScenes(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); got == "" || !containsAll(got, "http_status=401", "Unauthorized", "req-scenes-401") {
		t.Fatalf("unexpected error = %v", err)
	}
}

func TestClientExecuteScene(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1.1/scenes/scene-1/execute" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") == "" || r.Header.Get("sign") == "" {
			t.Fatalf("missing auth headers: %#v", r.Header)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"statusCode": 100,
			"message":    "success",
			"body":       map[string]any{},
		})
	}))
	defer server.Close()

	out, err := newTestClient(server).ExecuteScene(context.Background(), "scene-1")
	if err != nil {
		t.Fatalf("ExecuteScene() error = %v", err)
	}
	if out["scene_id"] != "scene-1" {
		t.Fatalf("scene_id = %#v", out["scene_id"])
	}
	if out["http_status"] != http.StatusOK {
		t.Fatalf("http_status = %#v", out["http_status"])
	}
}

func TestClientExecuteSceneRequiresSceneID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be called")
	}))
	defer server.Close()

	if _, err := newTestClient(server).ExecuteScene(context.Background(), " "); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestClientExecuteSceneRejectsAPIFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("switchbot-request-id", "req-scene-190")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"statusCode": 190,
			"message":    "System error",
			"body":       map[string]any{},
		})
	}))
	defer server.Close()

	_, err := newTestClient(server).ExecuteScene(context.Background(), "scene-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); got == "" || !containsAll(got, "status_code=190", "System error", "req-scene-190") {
		t.Fatalf("unexpected error = %v", err)
	}
}

func TestClientGetStatusRejectsUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("switchbot-request-id", "req-status-401")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": "Unauthorized",
		})
	}))
	defer server.Close()

	_, err := newTestClient(server).GetStatus(context.Background(), "", "hub2")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); got == "" || !containsAll(got, "http_status=401", "Unauthorized", "req-status-401") {
		t.Fatalf("unexpected error = %v", err)
	}
}

func containsAll(text string, substrings ...string) bool {
	for _, s := range substrings {
		if !strings.Contains(text, s) {
			return false
		}
	}
	return true
}
