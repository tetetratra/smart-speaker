package wschat

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/tetetratra/smart-speaker/internal/states/wakemode"
	"nhooyr.io/websocket"
)

// RegisterWakeModeは音声接続とは独立して、呼びかけ状態の同期と設定変更を扱う。
func RegisterWakeMode(mux *http.ServeMux, store *wakemode.Store) {
	mux.HandleFunc("/ws/wake-mode", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			http.Error(w, "待機モードを利用できません", http.StatusServiceUnavailable)
			return
		}
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		defer conn.CloseNow()
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		changed, unsubscribe := store.Subscribe()
		defer unsubscribe()
		results := make(chan string)
		go func() {
			defer cancel()
			for {
				_, data, err := conn.Read(ctx)
				if err != nil {
					return
				}
				var request struct {
					Type     string `json:"type"`
					Required *bool  `json:"required"`
				}
				message := ""
				if err := json.Unmarshal(data, &request); err != nil || request.Type != "set_wake_mode" || request.Required == nil {
					message = "set_wake_modeのrequiredにtrueまたはfalseを指定してください"
				} else if _, err := store.SetRequired(*request.Required, time.Now()); err != nil {
					message = "呼びかけ設定を保存できませんでした"
				}
				select {
				case results <- message:
				case <-ctx.Done():
					return
				}
			}
		}()

		// 状態の期限にだけタイマーを設定する。延長通知を受けたら最新の期限へ設定し直す。
		timer := time.NewTimer(time.Hour)
		defer timer.Stop()
		messageType, messageError := "wake_mode", ""
		for {
			timer.Stop()
			snapshot := store.Snapshot(time.Now())
			var expires <-chan time.Time
			if snapshot.AwakeUntil != nil {
				timer.Reset(time.Until(*snapshot.AwakeUntil))
				expires = timer.C
			}
			data, _ := json.Marshal(map[string]any{
				"type": messageType, "required": snapshot.Required, "awake": snapshot.Awake, "error": messageError,
			})
			writeCtx, stop := context.WithTimeout(ctx, 2*time.Second)
			err := conn.Write(writeCtx, websocket.MessageText, data)
			stop()
			if err != nil {
				return
			}
			messageType, messageError = "wake_mode", ""
			select {
			case <-ctx.Done():
				return
			case <-changed:
			case <-expires:
			case messageError = <-results:
				messageType = "wake_mode_result"
			}
		}
	})
}
