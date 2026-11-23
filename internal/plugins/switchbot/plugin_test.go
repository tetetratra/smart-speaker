package switchbot

import (
	"context"
	"encoding/json"
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
	p := New(plugin.Deps{})
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

// シーンがあればシーンのツールと指示文を出し、なければツールを出さない。
func TestPluginTools(t *testing.T) {
	client := NewSwitchbotClient("token", "secret")
	withScenes := &Plugin{client: client, scenes: []Scene{{SceneID: "scene-1", SceneName: "換気扇をつける"}}}
	if got := strings.Join(toolNames(withScenes), ","); got != "switchbot_execute_scene" {
		t.Fatalf("ツール = %s", got)
	}
	if !strings.Contains(withScenes.Instructions(), "scene_name") {
		t.Fatalf("指示文 = %q", withScenes.Instructions())
	}
	withoutScenes := &Plugin{client: client}
	if got := strings.Join(toolNames(withoutScenes), ","); got != "" {
		t.Fatalf("ツール = %s", got)
	}
	if withoutScenes.Instructions() != "" {
		t.Fatalf("シーンがないのに指示文が出ています: %q", withoutScenes.Instructions())
	}
}

// Hub 2 だけを対象にし、複数台あれば API の一覧で最初の機器を選ぶ。
func TestFindHub2(t *testing.T) {
	for _, tt := range []struct {
		name    string
		devices []Device
		fail    bool
		want    string
	}{
		{name: "Hub 2", devices: []Device{{DeviceID: "hub", DeviceType: "Hub 2"}}, want: "hub"},
		{name: "複数なら最初", devices: []Device{{DeviceID: "meter", DeviceType: "Meter"}, {DeviceID: "first", DeviceType: "Hub 2"}, {DeviceID: "second", DeviceType: "Hub 2"}}, want: "first"},
		{name: "対象なし", devices: []Device{{DeviceID: "motion", DeviceType: "Motion Sensor"}}},
		{name: "一覧取得失敗", fail: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.fail {
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"statusCode":190,"message":"error"}`))
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"statusCode": 100, "body": map[string]any{"deviceList": tt.devices}})
			}))
			defer server.Close()
			if got := findHub2(context.Background(), newTestClient(server)); got != tt.want {
				t.Fatalf("Hub 2 = %q、期待値 = %q", got, tt.want)
			}
		})
	}
}
