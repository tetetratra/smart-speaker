// Package switchbot は、SwitchBot の機器を扱うプラグイン。
//
// 登録済みのシーンを実行する AI ツールを持つ。
// Hub 2 の計測値と人感センサーの変化を、購読できる記録として会話へ入れる。
// Webhook 未設定なら Hub 2 を1分おきに取得する。
// 画面のリモコン（web/src/plugins/switchbot）向けに、シーンの一覧・実行の HTTP API も持つ。
//
// トークンとシークレットは環境変数 SWITCHBOT_TOKEN・SWITCHBOT_SECRET で渡す。
// Hub 2 は起動時にデバイス一覧から自動で見つける。
package switchbot

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/tetetratra/smart-speaker/internal/plugin"
	"github.com/tetetratra/smart-speaker/internal/tools"
)

const name = "switchbot"

// Plugin は SwitchBot のプラグイン。トークンかシークレットがなければ、ツールを出さない。
type Plugin struct {
	client *Client
	scenes []Scene
	// watching は Hub 2 の計測値を監視しているか。
	watching       bool
	motionWatching bool
	receiver       *webhookReceiver
}

// New は SwitchBot のプラグインを作る。有効なら、起動時にシーンの一覧を取得する。
// デバイス一覧に Hub 2 があれば、計測値の監視を始める（deps.Deliver がなければ始めない）。
func New(deps plugin.Deps) (*Plugin, error) {
	// 未知のキーを黙って無視せず、受信ポートの設定だけを読む。
	var cfg struct {
		WebhookAddr string `yaml:"webhook_addr"`
	}
	if err := deps.DecodeConfig(name, &cfg); err != nil {
		return nil, err
	}
	token := strings.TrimSpace(os.Getenv("SWITCHBOT_TOKEN"))
	secret := strings.TrimSpace(os.Getenv("SWITCHBOT_SECRET"))
	if token == "" || secret == "" {
		log.Println("switchbot: token or secret not set; switchbot tools disabled")
		return &Plugin{}, nil
	}
	webhookToken := strings.TrimSpace(os.Getenv("SWITCHBOT_WEBHOOK_TOKEN"))
	if webhookToken != "" && !ValidWebhookToken(webhookToken) {
		return nil, fmt.Errorf("SWITCHBOT_WEBHOOK_TOKEN は32文字以上の英数字・ハイフン・アンダースコアで指定してください")
	}
	p := newPlugin(deps, NewSwitchbotClient(token, secret), webhookToken != "")
	if p.receiver != nil {
		if err := serveWebhook(deps.AppContext, webhookAddr(cfg.WebhookAddr), p.receiver.handler(webhookToken)); err != nil {
			return nil, err
		}
	}
	return p, nil
}

func newPlugin(deps plugin.Deps, client *Client, webhook bool) *Plugin {
	ctx := deps.AppContext
	if ctx == nil {
		ctx = context.Background()
	}
	p := &Plugin{client: client, scenes: loadScenes(ctx, client)}
	if deps.Deliver != nil {
		hubID, motionID := findSensors(ctx, client, webhook)
		p.watching = hubID != ""
		p.motionWatching = motionID != ""
		if webhook {
			p.receiver = newWebhookReceiver(client, hubID, motionID, deps.Deliver)
			p.receiver.initialize(ctx)
		} else if p.watching {
			go newSensorWatcher(client, hubID, deps.Deliver).run(ctx, sensorInterval)
		}
	}
	return p
}

// findSensors は種類ごとに一覧の最初の機器を選ぶ。人感センサーは Webhook 使用時だけ対象にする。
func findSensors(parent context.Context, client *Client, motion bool) (string, string) {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	devices, err := client.ListDevices(ctx)
	if err != nil {
		log.Printf("switchbot: デバイス一覧を取得できないため、監視を開始しません: %v", err)
		return "", ""
	}
	selectDevice := func(deviceType string) string {
		var selected Device
		count := 0
		for _, device := range devices {
			if device.DeviceType != deviceType || strings.TrimSpace(device.DeviceID) == "" {
				continue
			}
			if count == 0 {
				selected = device
			}
			count++
		}
		if count == 0 {
			log.Printf("switchbot: %s が見つからないため、監視を開始しません", deviceType)
			return ""
		}
		log.Printf("switchbot: %s を%d台検出し、最初の1台を監視します: name=%q device_id=%q", deviceType, count, selected.DeviceName, selected.DeviceID)
		return selected.DeviceID
	}
	hubID := selectDevice("Hub 2")
	if motion {
		return hubID, selectDevice("Motion Sensor")
	}
	return hubID, ""
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
	sources := map[string]string{}
	if p.watching {
		sources[TemperatureSource] = "Hub 2 の室温が1℃刻みで変わった（event.metadata.temperature に計測値の数値）。サーバー起動直後にも現在の値が1件届く"
		sources[HumiditySource] = "Hub 2 の湿度が5%刻みで変わった（event.metadata.humidity に計測値の数値）。サーバー起動直後にも現在の値が1件届く"
		sources[LightSource] = "Hub 2 で測った部屋の明るさが2段階以上変わった（event.metadata.light_level に明るさ1〜20）。サーバー起動直後にも現在の値が1件届く"
	}
	if p.motionWatching {
		sources[MotionSource] = "人感センサーで人の動きの検知状態が変わった（event.metadata.move_detected に真偽値）。サーバー起動直後にも現在の状態が1件届く"
	}
	return sources
}

// Instructions は、シーンを実行するツールがあるときだけ、シーン名の選び方を伝える。
func (p *Plugin) Instructions() string {
	if NewScene(p.client, p.scenes) == nil {
		return ""
	}
	return "## SwitchBot\n- SwitchBotのscene_nameは登録済み候補から完全一致で選び、機器名だけに省略しないでください。例：「エアコン消して」には、候補にある「【エアコン】オフ」を使います。"
}
