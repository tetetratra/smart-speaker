package switchbot

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tetetratra/smart-speaker/internal/plugin"
)

func toolNames(p *Plugin) []string {
	var names []string
	for _, t := range p.Tools() {
		names = append(names, t.Name())
	}
	return names
}

// トークンとシークレットがなければツールも指示文も出さず、リモコンの API は 503 を返す。
func TestPluginWithoutCredentials(t *testing.T) {
	t.Setenv("SWITCHBOT_TOKEN", "")
	t.Setenv("SWITCHBOT_SECRET", "")
	p, err := New(plugin.Deps{})
	if err != nil {
		t.Fatal(err)
	}
	if names := toolNames(p); len(names) != 0 {
		t.Fatalf("ツールが出ています: %v", names)
	}
	if p.Instructions() != "" {
		t.Fatalf("指示文が出ています: %q", p.Instructions())
	}
	mux := http.NewServeMux()
	p.RegisterHTTP(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/switchbot/scenes", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d、期待値 = %d", rec.Code, http.StatusServiceUnavailable)
	}
}

// シーンがあればシーンのツールと指示文を出し、なければ Hub 2 のツールだけを出す。
func TestPluginTools(t *testing.T) {
	client := NewSwitchbotClient("token", "secret", map[string]string{"hub2": "hub-device"})
	withScenes := &Plugin{client: client, scenes: []Scene{{SceneID: "scene-1", SceneName: "換気扇をつける"}}}
	if got := strings.Join(toolNames(withScenes), ","); got != "switchbot_execute_scene,hub2_get_temperature,hub2_get_humidity,hub2_get_light_level" {
		t.Fatalf("ツール = %s", got)
	}
	if !strings.Contains(withScenes.Instructions(), "scene_name") {
		t.Fatalf("指示文 = %q", withScenes.Instructions())
	}
	withoutScenes := &Plugin{client: client}
	if got := strings.Join(toolNames(withoutScenes), ","); got != "hub2_get_temperature,hub2_get_humidity,hub2_get_light_level" {
		t.Fatalf("ツール = %s", got)
	}
	if withoutScenes.Instructions() != "" {
		t.Fatalf("シーンがないのに指示文が出ています: %q", withoutScenes.Instructions())
	}
}

// config.yaml の plugins.switchbot を読み、書き間違いはエラーにする。
func TestPluginReadsConfig(t *testing.T) {
	t.Setenv("SWITCHBOT_TOKEN", "")
	var gotName string
	deps := plugin.Deps{Config: func(name string, v any) error {
		gotName = name
		v.(*Config).DeviceMap = map[string]string{"hub2": "id"}
		return nil
	}}
	if _, err := New(deps); err != nil || gotName != "switchbot" {
		t.Fatalf("name = %q, err = %v", gotName, err)
	}
	deps.Config = func(string, any) error { return http.ErrNotSupported }
	if _, err := New(deps); err == nil {
		t.Fatal("設定の読み込みの失敗がエラーになりません")
	}
}
