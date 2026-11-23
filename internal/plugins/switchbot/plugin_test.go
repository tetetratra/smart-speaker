package switchbot

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

// config.yaml の plugins.switchbot を読み、書き間違いはエラーにする。
func TestPluginReadsConfig(t *testing.T) {
	t.Setenv("SWITCHBOT_TOKEN", "")
	var gotName string
	deps := plugin.Deps{Config: func(name string, v any) error {
		gotName = name
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

// Hub 2 の自動検出結果に応じて監視と購読元を公開し、シーンは監視と独立して使える。
func TestPluginWatchesHub2(t *testing.T) {
	for _, tt := range []struct {
		name      string
		devices   []Device
		fail      bool
		noDeliver bool
		wantID    string
	}{
		{name: "Hub 2", devices: []Device{{DeviceID: "hub-device", DeviceType: "Hub 2"}}, wantID: "hub-device"},
		{name: "複数なら最初", devices: []Device{{DeviceID: "meter", DeviceType: "Meter"}, {DeviceID: "first", DeviceName: "居間", DeviceType: "Hub 2"}, {DeviceID: "second", DeviceType: "Hub 2"}}, wantID: "first"},
		{name: "対象なし", devices: []Device{{DeviceID: "motion", DeviceType: "Motion Sensor"}}},
		{name: "一覧取得失敗", fail: true},
		{name: "届け先なし", devices: []Device{{DeviceID: "hub-device", DeviceType: "Hub 2"}}, noDeliver: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			previous := log.Writer()
			log.SetOutput(&logs)
			defer log.SetOutput(previous)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1.1/scenes":
					_ = json.NewEncoder(w).Encode(map[string]any{"statusCode": 100, "body": []map[string]string{{"sceneId": "scene", "sceneName": "照明"}}})
				case "/v1.1/devices":
					if tt.noDeliver {
						t.Error("届け先なしでデバイス一覧を取得しています")
					}
					if tt.fail {
						w.WriteHeader(http.StatusInternalServerError)
						_, _ = w.Write([]byte(`{"statusCode":190,"message":"error"}`))
						return
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"statusCode": 100, "body": map[string]any{"deviceList": tt.devices}})
				default:
					if tt.wantID == "" || r.URL.Path != "/v1.1/devices/"+tt.wantID+"/status" {
						t.Errorf("想定外のリクエスト: %s", r.URL.Path)
					}
					_, _ = w.Write([]byte(`{"statusCode":100,"body":{"temperature":25,"humidity":45,"lightLevel":4}}`))
				}
			}))
			defer server.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			records := make(chan types.ConversationInput, 3)
			deps := plugin.Deps{AppContext: ctx}
			if !tt.noDeliver {
				deps.Deliver = func(_ context.Context, input types.ConversationInput) error {
					records <- input
					return nil
				}
			}
			p := newPlugin(deps, newTestClient(server))
			if got := strings.Join(toolNames(p), ","); got != "switchbot_execute_scene" {
				t.Fatalf("監視の有無にかかわらずシーンを使えるはずです: %s", got)
			}
			sources := p.SubscribableSources()
			if tt.wantID == "" {
				if len(sources) != 0 {
					t.Fatalf("監視しないのに Sources = %#v", sources)
				}
				return
			}
			if len(sources) != 3 || sources[TemperatureSource] == "" || sources[HumiditySource] == "" || sources[LightSource] == "" {
				t.Fatalf("Sources = %#v", sources)
			}
			if tt.wantID == "first" && !strings.Contains(logs.String(), "2台検出") {
				t.Fatalf("複数検出のログがありません: %s", logs.String())
			}
			for range 3 {
				select {
				case input := <-records:
					if !input.Subscribable {
						t.Fatal("購読できない記録です")
					}
				case <-time.After(time.Second):
					t.Fatal("起動直後の計測値が届きません")
				}
			}
		})
	}
}
