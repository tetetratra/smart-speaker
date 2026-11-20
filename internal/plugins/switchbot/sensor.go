package switchbot

import (
	"context"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/tetetratra/smart-speaker/internal/types"
)

// 温度・湿度・明るさの変化の記録の Source。予約ジョブの schedule.source に指定できる。
const (
	TemperatureSource = "switchbot_temperature"
	HumiditySource    = "switchbot_humidity"
	LightSource       = "switchbot_light"
)

const (
	hub2Alias = "hub2"
	// sensorInterval は Hub 2 の計測値を取りに行く間隔。SwitchBot API の上限は1日10,000回。
	sensorInterval = 5 * time.Minute
)

// measurement は、Hub 2 の計測値1つを段階に分けて記録にする方法。
type measurement struct {
	source  string
	bodyKey string
	// classify は計測値を段階に分ける。段階は計測値が大きいほど大きくする（ヒステリシスで使う）。
	classify func(v float64) int
	// margin は、段階の境目をどれだけ越えたら次の段階に切り替えるか（境目付近でのばたつきを抑える）。
	margin float64
	// record は段階と計測値から記録の Title・Text・Metadata を作る。first は起動後の最初の1件。
	record func(level int, v float64, first bool) (title, text string, metadata map[string]any)
}

var measurements = []measurement{
	{
		source:   TemperatureSource,
		bodyKey:  "temperature",
		classify: func(v float64) int { return int(math.Round(v)) },
		margin:   0.3,
		record: func(level int, v float64, first bool) (string, string, map[string]any) {
			text := fmt.Sprintf("室温が%d℃になりました。", level)
			if first {
				text = fmt.Sprintf("現在の室温は%d℃です。", level)
			}
			return fmt.Sprintf("室温 %d℃", level), text, map[string]any{"temperature": v}
		},
	},
	{
		source:   HumiditySource,
		bodyKey:  "humidity",
		classify: func(v float64) int { return int(math.Round(v/5) * 5) },
		margin:   1,
		record: func(level int, v float64, first bool) (string, string, map[string]any) {
			text := fmt.Sprintf("湿度が%d%%になりました。", level)
			if first {
				text = fmt.Sprintf("現在の湿度は%d%%です。", level)
			}
			return fmt.Sprintf("湿度 %d%%", level), text, map[string]any{"humidity": v}
		},
	},
	{
		source:  LightSource,
		bodyKey: "lightLevel",
		// Hub 2 の lightLevel（1〜20）を2段階刻みにする（1・2→2、3・4→4、…）。
		classify: func(v float64) int { return int(math.Round(v/2) * 2) },
		margin:   1,
		record: func(level int, v float64, first bool) (string, string, map[string]any) {
			text := fmt.Sprintf("部屋の明るさが%d/20になりました。", level)
			if first {
				text = fmt.Sprintf("現在の部屋の明るさは%d/20です。", level)
			}
			return fmt.Sprintf("明るさ %d/20", level), text, map[string]any{"light_level": v}
		},
	},
}

// levelWithHysteresis は計測値の段階を返す。前の段階があれば、境目を margin 以上越えたときだけ切り替える。
func levelWithHysteresis(prev *int, v, margin float64, classify func(float64) int) int {
	level := classify(v)
	if prev == nil || level == *prev {
		return level
	}
	// 前の段階の側へ margin だけ戻しても前の段階に入らないときだけ切り替える。
	shifted := v + margin
	if level > *prev {
		shifted = v - margin
	}
	if classify(shifted) == *prev {
		return *prev
	}
	return level
}

// sensorWatcher は Hub 2 の計測値を定期的に取得し、段階が変わった計測値ごとに記録を会話へ入れる。
type sensorWatcher struct {
	client  *Client
	deliver func(context.Context, types.ConversationInput) error
	// levels は Source ごとの最後に届けた段階。メモリ上だけに持つ。
	levels map[string]int
	now    func() time.Time
}

func newSensorWatcher(client *Client, deliver func(context.Context, types.ConversationInput) error) *sensorWatcher {
	return &sensorWatcher{client: client, deliver: deliver, levels: map[string]int{}, now: time.Now}
}

// run は起動直後に1回取得し、そのあと interval ごとに取得する。ctx が取り消されたら終わる。
func (w *sensorWatcher) run(ctx context.Context, interval time.Duration) {
	w.check(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.check(ctx)
		}
	}
}

// check は計測値を1回取得し、段階が変わった計測値（起動後の最初はすべて）の記録を会話へ入れる。
// 取得に失敗したときはログに残すだけにし、次の取得で前回の段階と比べる。
func (w *sensorWatcher) check(ctx context.Context) {
	status, err := w.client.GetStatus(ctx, "", hub2Alias)
	if err != nil {
		log.Printf("switchbot: Hub 2 の計測値を取得できません: %v", err)
		return
	}
	body, _ := status["body"].(map[string]any)
	for _, m := range measurements {
		v, ok := body[m.bodyKey].(float64)
		if !ok {
			continue
		}
		prevLevel, seen := w.levels[m.source]
		var prev *int
		if seen {
			prev = &prevLevel
		}
		level := levelWithHysteresis(prev, v, m.margin, m.classify)
		if seen && level == prevLevel {
			continue
		}
		w.levels[m.source] = level
		title, text, metadata := m.record(level, v, !seen)
		input := types.ConversationInput{
			Record: types.ConversationRecord{
				ID:        "switchbot/" + uuid.NewString(),
				Role:      types.RoleSystem,
				Source:    m.source,
				Title:     title,
				Text:      text,
				Metadata:  metadata,
				CreatedAt: w.now(),
				External:  true,
			},
			Subscribable: true,
		}
		if err := w.deliver(ctx, input); err != nil {
			log.Printf("switchbot: 計測値の変化を会話へ届けられません: %v", err)
		}
	}
}
