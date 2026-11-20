// Package switchbot は、SwitchBot の機器を扱うプラグイン。
//
// 登録済みのシーンを実行する AI ツールを持つ。
// Hub 2 の温度・湿度・明るさは5分おきに取得し、段階が変わったら計測値ごとに購読できる記録として会話へ入れる。
// 画面のリモコン（web/src/plugins/switchbot）向けに、シーンの一覧・実行の HTTP API も持つ。
//
// トークンとシークレットは環境変数 SWITCHBOT_TOKEN・SWITCHBOT_SECRET で渡す。
// 機器名とデバイスIDの対応は config.yaml の plugins.switchbot.device_map に書く。
package switchbot

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/tetetratra/smart-speaker/internal/plugin"
	"github.com/tetetratra/smart-speaker/internal/tools"
)

const name = "switchbot"

// Config は config.yaml の plugins.switchbot。
type Config struct {
	// DeviceMap は機器名と SwitchBot のデバイスIDの対応。
	DeviceMap map[string]string `yaml:"device_map"`
}

// Plugin は SwitchBot のプラグイン。トークンかシークレットがなければ、ツールを出さない。
type Plugin struct {
	client *Client
	scenes []Scene
	// watching は Hub 2 の計測値を監視しているか。
	watching bool
}

// New は SwitchBot のプラグインを作る。有効なら、起動時にシーンの一覧を取得する。
// device_map に hub2 があれば、計測値の監視を始める（deps.Deliver がなければ始めない）。
func New(deps plugin.Deps) (*Plugin, error) {
	var cfg Config
	if err := deps.DecodeConfig(name, &cfg); err != nil {
		return nil, err
	}
	token := strings.TrimSpace(os.Getenv("SWITCHBOT_TOKEN"))
	secret := strings.TrimSpace(os.Getenv("SWITCHBOT_SECRET"))
	if token == "" || secret == "" {
		log.Println("switchbot: token or secret not set; switchbot tools disabled")
		return &Plugin{}, nil
	}
	client := NewSwitchbotClient(token, secret, cfg.DeviceMap)
	p := &Plugin{client: client, scenes: loadScenes(deps.AppContext, client)}
	if client.deviceMap[hub2Alias] != "" && deps.Deliver != nil {
		ctx := deps.AppContext
		if ctx == nil {
			ctx = context.Background()
		}
		p.watching = true
		go newSensorWatcher(client, deps.Deliver).run(ctx, sensorInterval)
	}
	return p, nil
}

func loadScenes(parent context.Context, client *Client) []Scene {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	scenes, err := client.ListScenes(ctx)
	if err != nil {
		log.Printf("switchbot: failed to list scenes; switchbot_execute_scene disabled: %v", err)
		return nil
	}
	return scenes
}

func (p *Plugin) Name() string { return name }

func (p *Plugin) Tools() []tools.Handler {
	var handlers []tools.Handler
	if scene := NewScene(p.client, p.scenes); scene != nil {
		handlers = append(handlers, scene)
	}
	return handlers
}

func (p *Plugin) RegisterHTTP(mux *http.ServeMux) {
	// client が nil のときは、型付きの nil ではなく nil の interface を渡し、API が 503 を返すようにする。
	var client switchBotSceneClient
	if p.client != nil {
		client = p.client
	}
	registerSwitchBotSceneAPI(mux, client)
}

func (p *Plugin) SubscribableSources() map[string]string {
	if !p.watching {
		return nil
	}
	return map[string]string{
		TemperatureSource: "Hub 2 の室温が1℃刻みで変わった（event.metadata.temperature に計測値の数値）。サーバー起動直後にも現在の値が1件届く",
		HumiditySource:    "Hub 2 の湿度が5%刻みで変わった（event.metadata.humidity に計測値の数値）。サーバー起動直後にも現在の値が1件届く",
		LightSource:       "Hub 2 で測った部屋の明るさが2段階以上変わった（event.metadata.light_level に明るさ1〜20。照明を消すと小さく、明るくつけると大きくなる）。サーバー起動直後にも現在の値が1件届く",
	}
}

// Instructions は、シーンを実行するツールがあるときだけ、シーン名の選び方を伝える。
func (p *Plugin) Instructions() string {
	if NewScene(p.client, p.scenes) == nil {
		return ""
	}
	return "## SwitchBot\n- SwitchBotのscene_nameは登録済み候補から完全一致で選び、機器名だけに省略しないでください。例：「エアコン消して」には、候補にある「【エアコン】オフ」を使います。"
}
