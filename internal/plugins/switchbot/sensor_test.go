package switchbot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/tetetratra/smart-speaker/internal/types"
)

// 起動後の最初は3つとも届け、そのあとは段階が変わった計測値だけを、計測値ごとの記録として届ける。
func TestSensorWatcherDeliversChangedLevels(t *testing.T) {
	var body map[string]any
	fail := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1.1/devices/hub-device/status" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if fail {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"statusCode": 190, "message": "error"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"statusCode": 100, "message": "success", "body": body})
	}))
	defer server.Close()

	var got []types.ConversationInput
	watcher := newSensorWatcher(newTestClient(server), "hub-device", func(_ context.Context, input types.ConversationInput) error {
		got = append(got, input)
		return nil
	})
	check := func(b map[string]any, want ...string) {
		t.Helper()
		body = b
		got = nil
		watcher.check(context.Background())
		var texts []string
		for _, input := range got {
			texts = append(texts, input.Record.Text)
		}
		if !reflect.DeepEqual(texts, want) {
			t.Fatalf("届いた記録 = %q、期待値 = %q", texts, want)
		}
	}

	check(map[string]any{"temperature": 24.6, "humidity": 46.0, "lightLevel": 3.0},
		"現在の室温は25℃です。", "現在の湿度は45%です。", "現在の部屋の明るさは4/20です。")
	first := got[0]
	if first.Record.Source != TemperatureSource || first.Record.Title != "室温 25℃" || !first.Subscribable ||
		first.ModelShouldRespond || first.ModelAlreadyKnows || first.UserAlreadyKnows || !first.Record.External ||
		first.Record.Role != types.RoleSystem || first.Record.Metadata["temperature"] != 24.6 {
		t.Fatalf("温度の記録 = %#v", first)
	}
	if light := got[2].Record; light.Source != LightSource || light.Title != "明るさ 4/20" || light.Metadata["light_level"] != 3.0 {
		t.Fatalf("明るさの記録 = %#v", light)
	}

	// 境目を越えても余裕の幅に収まるうちは届けない。
	check(map[string]any{"temperature": 25.7, "humidity": 48.4, "lightLevel": 5.0})
	check(map[string]any{"temperature": 24.2, "humidity": 41.6, "lightLevel": 3.0})
	// 余裕の幅を越えたものだけ届ける。
	check(map[string]any{"temperature": 25.8, "humidity": 48.4, "lightLevel": 6.0},
		"室温が26℃になりました。", "部屋の明るさが6/20になりました。")
	check(map[string]any{"temperature": 25.8, "humidity": 48.5, "lightLevel": 7.0}, "湿度が50%になりました。")

	// 取得に失敗したら何も届けず、次の取得で前回の段階と比べる。
	fail = true
	check(nil)
	fail = false
	check(map[string]any{"temperature": 25.8, "humidity": 50.0, "lightLevel": 20.0}, "部屋の明るさが20/20になりました。")

	// 計測値がない項目は飛ばす。
	check(map[string]any{"temperature": 30.0}, "室温が30℃になりました。")
}

func TestLevelWithHysteresis(t *testing.T) {
	round := measurements[0].classify
	prev := 25
	for _, tt := range []struct {
		v    float64
		want int
	}{
		{25.7, 25}, {25.8, 26}, {24.2, 25}, {24.1, 24}, {30, 30}, {20.4, 20},
	} {
		if got := levelWithHysteresis(&prev, tt.v, 0.3, round); got != tt.want {
			t.Errorf("前が25℃で %v℃ → %d、期待値 %d", tt.v, got, tt.want)
		}
	}
	if got := levelWithHysteresis(nil, 24.5, 0.3, round); got != 25 {
		t.Errorf("前がないとき 24.5℃ → %d、期待値 25", got)
	}
}
