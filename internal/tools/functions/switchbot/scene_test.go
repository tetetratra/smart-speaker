package switchbot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestSceneToolDefinition(t *testing.T) {
	tool := NewScene(&Client{}, []Scene{{SceneID: "scene-1", SceneName: "換気扇をつける"}, {SceneID: "scene-2", SceneName: "【エアコン】オフ"}})
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
	property := props["scene_name"].(map[string]any)
	if !reflect.DeepEqual(property["enum"], []string{"換気扇をつける", "【エアコン】オフ"}) {
		t.Fatalf("登録済みシーンの候補が一致しません: %#v", property["enum"])
	}
}

func TestSceneToolRun(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.URL.Path != "/v1.1/scenes/scene-1/execute" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"statusCode": 100,
			"message":    "success",
			"body":       map[string]any{},
		})
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

func TestSceneToolCanCorrectNameAfterValidationWithoutExecutingWrongScene(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{"statusCode": 100, "message": "success", "body": map[string]any{}})
	}))
	defer server.Close()
	tool := NewScene(newTestClient(server), []Scene{{SceneID: "aircon-off", SceneName: "【エアコン】オフ"}})
	_, err := tool.Run(map[string]any{"scene_name": "エアコン"})
	if err == nil || !strings.Contains(err.Error(), "外部操作は実行していません") || !strings.Contains(err.Error(), "【エアコン】オフ") {
		t.Fatalf("引数エラーに未実行と修正候補がありません: %v", err)
	}
	if len(paths) != 0 {
		t.Fatalf("誤ったシーン名で外部操作が実行されました: %v", paths)
	}
	if _, err := tool.Run(map[string]any{"scene_name": "【エアコン】オフ"}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(paths, []string{"/v1.1/scenes/aircon-off/execute"}) {
		t.Fatalf("正しいシーンが一度だけ実行されていません: %v", paths)
	}
}

func TestSceneToolRunRequiresSceneName(t *testing.T) {
	tool := NewScene(&Client{}, []Scene{{SceneID: "scene-1", SceneName: "換気扇をつける"}})
	if _, err := tool.Run(map[string]any{}); err == nil {
		t.Fatal("expected error, got nil")
	}
}
