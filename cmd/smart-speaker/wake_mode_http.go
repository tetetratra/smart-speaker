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
			Required      *bool    `json:"required"`
			Patterns      []string `json:"patterns"`
			MinConfidence *float64 `json:"min_confidence"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil || req.Required == nil {
			http.Error(w, "requiredにtrueまたはfalseを指定してください", 400)
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			http.Error(w, "JSONは1件だけ指定してください", 400)
			return
		}
		mode, err := store.SetOptions(*req.Required, req.Patterns, req.MinConfidence, time.Now())
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		writeJSON(w, 200, mode)
	})
}
