package switchbot

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tetetratra/smart-speaker/internal/plugin"
	"github.com/tetetratra/smart-speaker/internal/types"
)

var testWebhookToken = strings.Repeat("a", 64)

func eventJSON(deviceType, deviceID string, sample int64, values map[string]any) string {
	c := map[string]any{"deviceType": deviceType, "deviceMac": deviceID, "timeOfSample": sample}
	for k, v := range values {
		c[k] = v
	}
	body, _ := json.Marshal(map[string]any{"eventType": "changeReport", "eventVersion": "1", "context": c})
	return string(body)
}

func postEvent(handler http.Handler, body string) int {
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/"+testWebhookToken, strings.NewReader(body)))
	return w.Code
}

func TestWebhookRecordsAndOrdering(t *testing.T) {
	var records []types.ConversationInput
	receiver := newWebhookReceiver(nil, "HUB", "MOTION", func(_ context.Context, in types.ConversationInput) error { records = append(records, in); return nil })
	handler := receiver.handler(testWebhookToken)
	hub := func(sample int64, temperature float64) string {
		return eventJSON("WoHub2", "hub", sample, map[string]any{"temperature": temperature, "humidity": 45, "lightLevel": 4, "scale": "CELSIUS"})
	}
	if code := postEvent(handler, hub(100, 25)); code != http.StatusNoContent {
		t.Fatal(code)
	}
	if len(records) != 3 {
		t.Fatalf("初回 = %d件", len(records))
	}
	// 同じ状態・重複・古い通知を受けても、記録を増やしたり状態を戻したりしない。
	postEvent(handler, hub(100, 25))
	postEvent(handler, hub(101, 25.1))
	postEvent(handler, hub(99, 20))
	if len(records) != 3 {
		t.Fatalf("変化なしで %d件", len(records))
	}
	postEvent(handler, hub(102, 26))
	if len(records) != 4 || records[3].Record.Source != TemperatureSource {
		t.Fatalf("温度の変化 = %#v", records)
	}
	for _, state := range []string{"NOT_DETECTED", "DETECTED", "DETECTED", "NOT_DETECTED"} {
		postEvent(handler, eventJSON("WoPresence", "motion", 200, map[string]any{"detectionState": state}))
	}
	if len(records) != 7 {
		t.Fatalf("人感の記録 = %d件", len(records)-4)
	}
	for _, in := range records[4:] {
		if in.Record.Source != MotionSource || !in.Subscribable || !in.UserAlreadyKnows || in.ModelAlreadyKnows || in.ModelShouldRespond {
			t.Fatalf("人感の配送条件 = %#v", in)
		}
	}
	if records[5].Record.Metadata["move_detected"] != true || records[6].Record.Metadata["move_detected"] != false {
		t.Fatal("検知状態が異なります")
	}
}

func TestWebhookRejectsInvalidRequests(t *testing.T) {
	delivered := 0
	receiver := newWebhookReceiver(nil, "hub", "motion", func(context.Context, types.ConversationInput) error { delivered++; return nil })
	handler := receiver.handler(testWebhookToken)
	valid := eventJSON("WoHub2", "hub", 1, map[string]any{"temperature": 25})
	for _, tt := range []struct {
		name, method, path, body string
		status                   int
	}{
		{"秘密なし", "POST", "/", valid, 404},
		{"画面のパス", "POST", "/api/switchbot/scenes", valid, 404},
		{"GET", "GET", "/" + testWebhookToken, valid, 405},
		{"壊れたJSON", "POST", "/" + testWebhookToken, "{", 400},
		{"連結JSON", "POST", "/" + testWebhookToken, valid + valid, 400},
		{"過大な本文", "POST", "/" + testWebhookToken, strings.Repeat(" ", 65<<10) + valid, 400},
		{"未選択機器", "POST", "/" + testWebhookToken, eventJSON("WoHub2", "other", 1, map[string]any{"temperature": 25}), 204},
		{"別の機種", "POST", "/" + testWebhookToken, eventJSON("WoHand", "hub", 1, map[string]any{"temperature": 25}), 204},
		{"時刻なし", "POST", "/" + testWebhookToken, eventJSON("WoHub2", "hub", 0, map[string]any{"temperature": 25}), 400},
		{"計測値なし", "POST", "/" + testWebhookToken, eventJSON("WoHub2", "hub", 1, nil), 400},
		{"湿度の範囲外", "POST", "/" + testWebhookToken, eventJSON("WoHub2", "hub", 1, map[string]any{"humidity": 101}), 400},
		{"明るさの範囲外", "POST", "/" + testWebhookToken, eventJSON("WoHub2", "hub", 1, map[string]any{"lightLevel": 0}), 400},
		{"温度の単位", "POST", "/" + testWebhookToken, eventJSON("WoHub2", "hub", 1, map[string]any{"temperature": 70, "scale": "FAHRENHEIT"}), 400},
		{"人感の未知の状態", "POST", "/" + testWebhookToken, eventJSON("WoPresence", "motion", 1, map[string]any{"detectionState": "unknown"}), 400},
	} {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body)))
			if w.Code != tt.status {
				t.Fatalf("status=%d、期待値=%d", w.Code, tt.status)
			}
		})
	}
	if delivered != 0 {
		t.Fatal("無効な通知が配送されました")
	}
}

func TestWebhookRetriesFailedDeliveryAndSerializesEvents(t *testing.T) {
	fail := true
	records := 0
	receiver := newWebhookReceiver(nil, "hub", "motion", func(context.Context, types.ConversationInput) error {
		if fail {
			return errors.New("配送失敗")
		}
		records++
		return nil
	})
	handler := receiver.handler(testWebhookToken)
	for _, body := range []string{eventJSON("WoHub2", "hub", 1, map[string]any{"temperature": 25}), eventJSON("WoPresence", "motion", 1, map[string]any{"detectionState": "DETECTED"})} {
		fail = true
		if code := postEvent(handler, body); code != 500 {
			t.Fatal(code)
		}
		fail = false
		if code := postEvent(handler, body); code != 204 {
			t.Fatal(code)
		}
	}
	if records != 2 {
		t.Fatalf("失敗した状態が再配送されません: %d", records)
	}
	// 同じ通知を同時に受けても一度だけ配送する（race検査の対象にもなる）。
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			postEvent(handler, eventJSON("WoPresence", "motion", 2, map[string]any{"detectionState": "NOT_DETECTED"}))
		}()
	}
	wg.Wait()
	if records != 3 {
		t.Fatalf("同時受信で重複配送: %d", records)
	}
}

func TestWebhookPluginInitializesSelectedSensors(t *testing.T) {
	requests := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests[r.URL.Path]++
		switch r.URL.Path {
		case "/v1.1/scenes":
			_, _ = w.Write([]byte(`{"statusCode":100,"body":[]}`))
		case "/v1.1/devices":
			_, _ = w.Write([]byte(`{"statusCode":100,"body":{"deviceList":[{"deviceId":"hub","deviceType":"Hub 2"},{"deviceId":"motion","deviceType":"Motion Sensor"},{"deviceId":"other-motion","deviceType":"Motion Sensor"}]}}`))
		case "/v1.1/devices/hub/status":
			_, _ = w.Write([]byte(`{"statusCode":100,"body":{"temperature":25,"humidity":45,"lightLevel":4}}`))
		case "/v1.1/devices/motion/status":
			_, _ = w.Write([]byte(`{"statusCode":100,"body":{"moveDetected":false}}`))
		default:
			t.Errorf("想定外の取得: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	var records []types.ConversationInput
	p := newPlugin(plugin.Deps{Deliver: func(_ context.Context, in types.ConversationInput) error { records = append(records, in); return nil }}, newTestClient(server), true)
	if p.receiver == nil || len(p.SubscribableSources()) != 4 || len(records) != 4 {
		t.Fatalf("初期化: sources=%#v records=%d", p.SubscribableSources(), len(records))
	}
	postEvent(p.receiver.handler(testWebhookToken), eventJSON("WoPresence", "motion", 1, map[string]any{"detectionState": "DETECTED"}))
	if len(records) != 5 {
		t.Fatal("初期化後の変化が届きません")
	}
	if requests["/v1.1/devices/hub/status"] != 1 || requests["/v1.1/devices/motion/status"] != 1 {
		t.Fatalf("通知時に再取得しています: %#v", requests)
	}
}

func TestWebhookListenerClosesWithContext(t *testing.T) {
	// ポートを予約してすぐ返し、専用サーバーを実際にloopbackで起動する。
	reservation, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := reservation.Addr().String()
	_ = reservation.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	handler := newWebhookReceiver(nil, "", "", nil).handler(testWebhookToken)
	if err := serveWebhook(ctx, addr, handler); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: time.Second}
	resp, err := client.Get("http://" + addr + "/api/jobs")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("既存APIを公開しています: %d", resp.StatusCode)
	}
	cancel()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 20*time.Millisecond)
		if err != nil {
			return
		}
		_ = conn.Close()
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("context取消後も待ち受けています")
}

func TestWebhookConfiguration(t *testing.T) {
	t.Setenv("SWITCHBOT_TOKEN", "token")
	t.Setenv("SWITCHBOT_SECRET", "secret")
	for _, value := range []string{"short", strings.Repeat("a", 32) + "/"} {
		t.Setenv("SWITCHBOT_WEBHOOK_TOKEN", value)
		if _, err := New(plugin.Deps{}); err == nil {
			t.Errorf("無効な秘密を受理しました: %q", value)
		}
	}
	if !ValidWebhookToken(testWebhookToken) {
		t.Fatal("有効な秘密を拒否しています")
	}
	if got := webhookAddr(""); got != "127.0.0.1:8083" {
		t.Fatalf("既定の待受け先=%s", got)
	}
}
