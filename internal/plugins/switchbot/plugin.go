// Package switchbot は、SwitchBot の機器を扱うプラグイン。
//
// 登録済みのシーンを実行する AI ツールを持つ。
// Hub 2 の温度・湿度・明るさは1分おきに取得し、段階が変わったら計測値ごとに購読できる記録として会話へ入れる。
// 画面のリモコン（web/src/plugins/switchbot）向けに、シーンの一覧・実行の HTTP API も持つ。
//
// トークンとシークレットは環境変数 SWITCHBOT_TOKEN・SWITCHBOT_SECRET で渡す。
// Hub 2 は起動時にデバイス一覧から自動で見つける。
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

// Plugin は SwitchBot のプラグイン。トークンかシークレットがなければ、ツールを出さない。
type Plugin struct {
	client *Client
	scenes []Scene
	// watching は Hub 2 の計測値を監視しているか。
	watching bool
}

// New は SwitchBot のプラグインを作る。有効なら、起動時にシーンの一覧を取得する。
// デバイス一覧に Hub 2 があれば、計測値の監視を始める（deps.Deliver がなければ始めない）。
func New(deps plugin.Deps) (*Plugin, error) {
	// 設定項目はないが、未知のキーを黙って無視しないよう検査する。
	var cfg struct{}
	if err := deps.DecodeConfig(name, &cfg); err != nil {
		return nil, err
	}
	token := strings.TrimSpace(os.Getenv("SWITCHBOT_TOKEN"))
	secret := strings.TrimSpace(os.Getenv("SWITCHBOT_SECRET"))
	if token == "" || secret == "" {
		log.Println("switchbot: token or secret not set; switchbot tools disabled")
		return &Plugin{}, nil
	}
	return newPlugin(deps, NewSwitchbotClient(token, secret)), nil
}

func newPlugin(deps plugin.Deps, client *Client) *Plugin {
	ctx := deps.AppContext
	if ctx == nil {
		ctx = context.Background()
	}
	p := &Plugin{client: client, scenes: loadScenes(ctx, client)}
	if deps.Deliver != nil {
		if deviceID := findHub2(ctx, client); deviceID != "" {
			p.watching = true
			go newSensorWatcher(client, deviceID, deps.Deliver).run(ctx, sensorInterval)
		}
	}
	return p
}

// findHub2 は一覧の最初の Hub 2 を選ぶ。複数あれば、採用した機器をログに残す。
func findHub2(parent context.Context, client *Client) string {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	devices, err := client.ListDevices(ctx)
	if err != nil {
		log.Printf("switchbot: デバイス一覧を取得できないため、Hub 2 の監視を開始しません: %v", err)
		return ""
	}
	var selected Device
	count := 0
	for _, device := range devices {
		if device.DeviceType != "Hub 2" || strings.TrimSpace(device.DeviceID) == "" {
			continue
		}
		if count == 0 {
			selected = device
		}
		count++
	}
	if count == 0 {
		log.Println("switchbot: Hub 2 が見つからないため、監視を開始しません")
		return ""
	}
	log.Printf("switchbot: Hub 2 を%d台検出し、一覧の最初の1台を監視します: name=%q device_id=%q", count, selected.DeviceName, selected.DeviceID)
	return selected.DeviceID
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

// Instructions は、有効なシーン操作とセンサー監視について、利用方法を伝える。
func (p *Plugin) Instructions() string {
	var sections []string
	if NewScene(p.client, p.scenes) != nil {
		sections = append(sections, "## SwitchBotの操作\n- SwitchBotのscene_nameは登録済み候補から完全一致で選び、機器名だけに省略しないでください。例：「エアコン消して」には、候補にある「【エアコン】オフ」を使います。")
	}
	if p.watching {
		sections = append(sections, sensorInstructions)
	}
	return strings.Join(sections, "\n\n")
}

const sensorInstructions = `## SwitchBotの室温・湿度・明るさ
- システムがHub 2の計測値を定期取得し、起動直後と、値の段階が変わったときに会話へ記録を届けます。室温は1℃、湿度は5%、明るさは2段階刻みです。毎回同じ値の記録が届く仕組みではありません。
- 初回の通知後は、室温・湿度・明るさそれぞれの段階に変化があった場合にだけ、その項目のイベントが届きます。逆に、新しいイベントが届いていない項目は、システムが把握する状態が前回から変わっていないという意味です。この扱いはこれらの状態通知に限り、イベント全般には適用しないでください。
- 記録の出どころはswitchbot_temperature（室温）、switchbot_humidity（湿度）、switchbot_light（部屋の明るさ）です。新しく届く記録は「室温が23℃になりました。」「湿度が50%になりました。」「部屋の明るさが12/20になりました。」などの本文や付随情報でも渡されます。これらは回答に使える計測情報です。
- 今の室温・湿度・明るさを聞かれたら、項目ごとに最後に届いた記録の値を、システムが把握している現在の値として答えてください。更新がなければ前回の値を引き続き使います。値が古いことや新着イベントがないことだけを理由に、不明・未受信として扱わないでください。
- 通常は値と単位だけを一文で答えてください。「ライブ取得できない」「今も同じとは限らない」「あくまで推定」といった留保や、手元で確認するよう促す説明は不要です。明示的に障害や欠測が通知されている場合は、その事実を伝えてください。
- ある項目の値が記録に一つもなければ、その項目だけ不明と伝え、ほかの分かる値は答えてください。メモリ内の回答例の数値を、実際の計測値として使わないでください。
- 明るさは20段階の値です。12/20など、記録にある尺度で答え、照明のオン・オフ、ルクス、パーセントに読み替えないでください。
- 以下は回答方法の例であり、実際の値はその会話の記録から選んでください。
  - 記録：室温23℃、湿度50%。以後の更新なし。質問：「今の温度と湿度は？」→「室温は23度、湿度は50パーセントだよ。」
  - 上の状態で湿度55%の記録を受信。質問：「今は？」→「室温は23度、湿度は55パーセントだよ。」
  - 記録：明るさ12/20。質問：「今の明るさは？」→「20段階で12だよ。」`
