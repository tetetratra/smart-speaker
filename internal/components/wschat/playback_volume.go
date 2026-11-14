package wschat

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/tetetratra/smart-speaker/internal/states/playbackvolume"
	"nhooyr.io/websocket"
)

// RegisterPlaybackVolumeは音声接続とは独立して、全端末で共通の再生音量の同期と変更を扱う。
// 接続時と変更時に現在の音量を送り、画面からの変更も全端末へ配る。
func RegisterPlaybackVolume(mux *http.ServeMux, store *playbackvolume.Store) {
	mux.HandleFunc("/ws/playback-volume", func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		defer conn.CloseNow()
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		changed, unsubscribe := store.Subscribe()
		defer unsubscribe()
		go func() {
			defer cancel()
			for {
				_, data, err := conn.Read(ctx)
				if err != nil {
					return
				}
				var request struct {
					Type  string `json:"type"`
					Level string `json:"level"`
				}
				// 不正な値は無視する。画面は次に届く現在の音量で表示を戻す。
				if json.Unmarshal(data, &request) == nil && request.Type == "set_playback_volume" {
					store.Set(request.Level)
				}
			}
		}()

		for {
			data, _ := json.Marshal(map[string]string{"type": "playback_volume", "level": store.Level()})
			writeCtx, stop := context.WithTimeout(ctx, 2*time.Second)
			err := conn.Write(writeCtx, websocket.MessageText, data)
			stop()
			if err != nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-changed:
			}
		}
	})
}
