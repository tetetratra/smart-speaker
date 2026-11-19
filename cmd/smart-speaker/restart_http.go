package main

import (
	"net/http"
	"time"
)

// restartDelay は、再起動を受け付けた応答を返してから、終了を始めるまでの待ち時間。
// すぐに終了を始めると、応答を返す前に HTTP サーバーが閉じてしまう。
const restartDelay = 500 * time.Millisecond

// registerRestartAPI は、サーバーを再起動する API を登録する。設定を変えたあとや MCP サーバーを認可したあとに、画面から使う。
func registerRestartAPI(mux *http.ServeMux, restart func()) {
	if restart == nil {
		return
	}
	mux.HandleFunc("POST /api/restart", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "restarting"})
		time.AfterFunc(restartDelay, restart)
	})
}
