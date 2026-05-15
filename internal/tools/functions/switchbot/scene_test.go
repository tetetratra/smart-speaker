package switchbot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSceneToolDefinition(t *testing.T) {
	tool := NewScene(&Client{}, []Scene{{SceneID: "scene-1", SceneName: "換気扇をつける"}})
	def := tool.Definition()

	if def["name"] != sceneToolName {
		t.Fatalf("name = %#v", def["name"])
	}
	desc, _ := def["description"].(string)
	if !strings.Contains(desc, "換気扇をつける") {
		t.Fatalf("description = %q", desc)
	}
	params, ok := def["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("parameters = %#v", def["parameters"])
	}
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %#v", params["properties"])
	}
	if _, ok := props["scene_name"]; !ok {
		t.Fatalf("scene_name property is missing: %#v", props)
	}
}

func TestSceneToolRun(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.URL.Path != "/v1.1/scenes/scene-1/execute" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "success"})
	}))
	defer server.Close()

	tool := NewScene(newTestClient(server), []Scene{{SceneID: "scene-1", SceneName: "換気扇をつける"}})
	tool.SetContext(context.Background())
	out, err := tool.Run(map[string]any{"scene_name": "換気扇をつける"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !called {
		t.Fatal("server was not called")
	}
	if out["scene_id"] != "scene-1" {
		t.Fatalf("out = %#v", out)
	}
}

func TestSceneToolRunRejectsUnknownScene(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be called")
	}))
	defer server.Close()

	tool := NewScene(newTestClient(server), []Scene{{SceneID: "scene-1", SceneName: "換気扇をつける"}})
	_, err := tool.Run(map[string]any{"scene_name": "未登録"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "換気扇をつける") {
		t.Fatalf("error = %v", err)
	}
}

func TestSceneToolRunRequiresSceneName(t *testing.T) {
	tool := NewScene(&Client{}, []Scene{{SceneID: "scene-1", SceneName: "換気扇をつける"}})
	if _, err := tool.Run(map[string]any{}); err == nil {
		t.Fatal("expected error, got nil")
	}
}
