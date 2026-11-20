package switchbot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tetetratra/smart-speaker/internal/plugin"
	"github.com/tetetratra/smart-speaker/internal/types"
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

// シーンがあればシーンのツールと指示文を出し、なければツールを出さない。
func TestPluginTools(t *testing.T) {
	client := NewSwitchbotClient("token", "secret", map[string]string{"hub2": "hub-device"})
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

// device_map に hub2 があり、会話へ届ける先があるときだけ、計測値を監視して購読できる記録を出す。
func TestPluginWatchesHub2(t *testing.T) {
	t.Setenv("SWITCHBOT_TOKEN", "token")
	t.Setenv("SWITCHBOT_SECRET", "secret")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	deliver := func(context.Context, types.ConversationInput) error { return nil }
	newPlugin := func(deviceMap map[string]string, deliver func(context.Context, types.ConversationInput) error) *Plugin {
		t.Helper()
		p, err := New(plugin.Deps{AppContext: ctx, Deliver: deliver, Config: func(_ string, v any) error {
			v.(*Config).DeviceMap = deviceMap
			return nil
		}})
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	// 取り消し済みの context で作り、シーンの取得と監視が本物の API へ行かないようにする。
	cancel()
	if got := newPlugin(map[string]string{"hub2": "id"}, deliver).SubscribableSources(); len(got) != 3 || got[TemperatureSource] == "" || got[HumiditySource] == "" || got[LightSource] == "" {
		t.Fatalf("Sources = %#v", got)
	}
	if got := newPlugin(nil, deliver).SubscribableSources(); len(got) != 0 {
		t.Fatalf("hub2 がないのに Sources = %#v", got)
	}
	if got := newPlugin(map[string]string{"hub2": "id"}, nil).SubscribableSources(); len(got) != 0 {
		t.Fatalf("届け先がないのに Sources = %#v", got)
	}
}
