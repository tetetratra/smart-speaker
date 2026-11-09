package main

import (
	"encoding/json"
	"github.com/tetetratra/smart-speaker/internal/states/wakemode"
	"io"
	"net/http"
	"time"
)

func registerWakeModeAPI(mux *http.ServeMux, store *wakemode.Store) {
	mux.HandleFunc("/api/wake-mode", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodGet && r.Method != http.MethodPut {
			w.Header().Set("Allow", "GET, PUT")
			w.WriteHeader(405)
			return
		}
		if store == nil {
			http.Error(w, "待機モードを利用できません", 503)
			return
		}
		if r.Method == http.MethodGet {
			writeJSON(w, 200, store.Snapshot(time.Now()))
			return
		}
		var req struct {
			Required *bool `json:"required"`
		}
		// 判定の言葉やしきい値はデプロイ時の環境変数で決めるため、ここでは必須モードだけを受け付ける。
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil || req.Required == nil {
			http.Error(w, "requiredにtrueまたはfalseを指定してください", 400)
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			http.Error(w, "JSONは1件だけ指定してください", 400)
			return
		}
		mode, err := store.SetRequired(*req.Required, time.Now())
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		writeJSON(w, 200, mode)
	})
	// 呼びかけの発話の代わりに、会話中（呼びかけなしで話せる状態）にする。
	// 呼びかけの音声認識が外れやすいため、スマホのショートカットなどから確実に話し始められるようにする。
	mux.HandleFunc("/api/wake-mode/wake", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			w.WriteHeader(405)
			return
		}
		if store == nil {
			http.Error(w, "待機モードを利用できません", 503)
			return
		}
		writeJSON(w, 200, store.Wake(time.Now()))
	})
}
