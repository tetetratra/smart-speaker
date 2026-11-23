package switchbot

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/tetetratra/smart-speaker/internal/types"
)

const MotionSource = "switchbot_motion"

// webhookReceiver は選択した機器の通知だけを受け、起動時の状態と同じ記録へ変換する。
// 最後の状態と時刻はメモリ上だけに持ち、再起動時には REST で現在の状態を取り直す。
type webhookReceiver struct {
	client     *Client
	hub        *sensorWatcher
	motionID   string
	deliver    func(context.Context, types.ConversationInput) error
	mu         sync.Mutex
	lastSample map[string]int64
	motion     *bool
}

func newWebhookReceiver(client *Client, hubID, motionID string, deliver func(context.Context, types.ConversationInput) error) *webhookReceiver {
	r := &webhookReceiver{client: client, motionID: motionID, deliver: deliver, lastSample: map[string]int64{}}
	if hubID != "" {
		r.hub = newSensorWatcher(client, hubID, deliver)
	}
	return r
}

// initialize は受信開始前に呼ぶ。REST の遅い応答が新しい通知を上書きする競合を避ける。
func (r *webhookReceiver) initialize(ctx context.Context) {
	if r.hub != nil {
		r.hub.check(ctx)
	}
	if r.motionID != "" {
		status, err := r.client.GetStatus(ctx, r.motionID)
		if err == nil {
			body, _ := status["body"].(map[string]any)
			detected, ok := body["moveDetected"].(bool)
			if !ok {
				err = errors.New("moveDetected がありません")
			} else {
				err = r.observeMotion(ctx, detected)
			}
		}
		if err != nil {
			log.Printf("switchbot: 人感センサーの初期状態を取得・配送できません: %v", err)
		}
	}
}

func (r *webhookReceiver) observeMotion(ctx context.Context, detected bool) error {
	if r.motion != nil && *r.motion == detected {
		return nil
	}
	text := "人の動きがなくなりました。"
	if detected {
		text = "人の動きを検知しました。"
	}
	if r.motion == nil {
		text = "現在、人の動きは検知していません。"
		if detected {
			text = "現在、人の動きを検知しています。"
		}
	}
	input := types.ConversationInput{
		Record:       types.ConversationRecord{ID: "switchbot/" + uuid.NewString(), Role: types.RoleSystem, Source: MotionSource, Title: "人感センサー", Text: text, Metadata: map[string]any{"move_detected": detected}, CreatedAt: time.Now(), External: true},
		Subscribable: true, UserAlreadyKnows: true,
	}
	if err := r.deliver(ctx, input); err != nil {
		return err
	}
	r.motion = &detected
	return nil
}

// ValidWebhookToken は秘密URLのパスへ使える、十分な長さの値かを返す。
func ValidWebhookToken(token string) bool {
	if len(token) < 32 {
		return false
	}
	for _, c := range token {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

type webhookEvent struct {
	EventType    string `json:"eventType"`
	EventVersion string `json:"eventVersion"`
	Context      struct {
		DeviceType     string   `json:"deviceType"`
		DeviceMAC      string   `json:"deviceMac"`
		TimeOfSample   int64    `json:"timeOfSample"`
		Temperature    *float64 `json:"temperature"`
		Humidity       *float64 `json:"humidity"`
		LightLevel     *float64 `json:"lightLevel"`
		Scale          string   `json:"scale"`
		DetectionState string   `json:"detectionState"`
	} `json:"context"`
}

// handler は秘密URLを持つ POST だけ受ける。公開仕様で通知署名を確認できていないため、
// URLを知る相手のなりすましを防げるとは扱わず、機器と通知の内容も検査する。
func (r *webhookReceiver) handler(token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if subtle.ConstantTimeCompare([]byte(req.URL.Path), []byte("/"+token)) != 1 {
			http.NotFound(w, req)
			return
		}
		if req.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var event webhookEvent
		decoder := json.NewDecoder(http.MaxBytesReader(w, req.Body, 64<<10))
		if err := decoder.Decode(&event); err != nil {
			http.Error(w, "invalid event", http.StatusBadRequest)
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			http.Error(w, "invalid event", http.StatusBadRequest)
			return
		}
		if event.EventType != "changeReport" || event.EventVersion != "1" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		c := event.Context
		// REST の deviceId と Webhook の deviceMac は表記の大小を揃えて比較する。
		hub := r.hub != nil && c.DeviceType == "WoHub2" && strings.EqualFold(c.DeviceMAC, r.hub.deviceID)
		motion := r.motionID != "" && c.DeviceType == "WoPresence" && strings.EqualFold(c.DeviceMAC, r.motionID)
		if !hub && !motion {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		body := map[string]any{}
		if c.TimeOfSample <= 0 {
			http.Error(w, "invalid sample time", http.StatusBadRequest)
			return
		}
		if hub {
			if c.Scale != "" && c.Scale != "CELSIUS" {
				http.Error(w, "unsupported scale", http.StatusBadRequest)
				return
			}
			if c.Temperature != nil {
				body["temperature"] = *c.Temperature
			}
			if c.Humidity != nil {
				if *c.Humidity < 0 || *c.Humidity > 100 {
					http.Error(w, "invalid humidity", http.StatusBadRequest)
					return
				}
				body["humidity"] = *c.Humidity
			}
			if c.LightLevel != nil {
				if *c.LightLevel < 1 || *c.LightLevel > 20 {
					http.Error(w, "invalid light level", http.StatusBadRequest)
					return
				}
				body["lightLevel"] = *c.LightLevel
			}
			if len(body) == 0 {
				http.Error(w, "missing measurements", http.StatusBadRequest)
				return
			}
		} else if c.DetectionState != "DETECTED" && c.DetectionState != "NOT_DETECTED" {
			http.Error(w, "invalid detection state", http.StatusBadRequest)
			return
		}
		r.mu.Lock()
		defer r.mu.Unlock()
		key := strings.ToUpper(c.DeviceMAC)
		if c.TimeOfSample < r.lastSample[key] {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		var err error
		if hub {
			err = r.hub.observe(req.Context(), body)
		} else {
			err = r.observeMotion(req.Context(), c.DetectionState == "DETECTED")
		}
		if err != nil {
			log.Printf("switchbot: Webhookの記録を配送できません: %v", err)
			http.Error(w, "delivery failed", http.StatusInternalServerError)
			return
		}
		r.lastSample[key] = c.TimeOfSample
		w.WriteHeader(http.StatusNoContent)
	})
}

func webhookAddr(configured string) string {
	if configured != "" {
		return configured
	}
	return "127.0.0.1:8083"
}

// serveWebhook は画面・操作APIを持たない専用サーバーを起動する。既定はloopback。
// Docker用に待受け先を変えるときも、ホスト側のポート公開はloopbackに限定する。
func serveWebhook(parent context.Context, addr string, handler http.Handler) error {
	if parent == nil {
		parent = context.Background()
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("switchbot: Webhook受信ポートを開けません: %w", err)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: time.Minute, MaxHeaderBytes: 8 << 10}
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("switchbot: Webhook受信を停止しました: %v", err)
		}
	}()
	go func() {
		select {
		case <-parent.Done():
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := server.Shutdown(ctx); err != nil {
				_ = server.Close()
			}
		case <-done:
		}
	}()
	return nil
}
