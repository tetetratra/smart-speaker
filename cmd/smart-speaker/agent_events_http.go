package main

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"

	"github.com/tetetratra/smart-speaker/internal/agentbridge"
)

// registerAgentEventsAPI は、手元のMacの agent-bridge から承認依頼・完了・失敗の通知を受け取る。
// 他のAPIと違い、Macで作業を進める入口になるので共有トークンで認証する。
func registerAgentEventsAPI(mux *http.ServeMux, service *agentbridge.Service, token string) {
	mux.HandleFunc("/api/agent-events", func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			http.Error(w, "Claude Code連携は設定されていません", http.StatusServiceUnavailable)
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "POSTで送ってください", http.StatusMethodNotAllowed)
			return
		}
		expected := []byte("Bearer " + token)
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), expected) != 1 {
			http.Error(w, "認証に失敗しました", http.StatusUnauthorized)
			return
		}
		var event agentbridge.Event
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256*1024))
		if err := decoder.Decode(&event); err != nil {
			http.Error(w, "有効なJSONを指定してください", http.StatusBadRequest)
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			http.Error(w, "JSONは1件だけ指定してください", http.StatusBadRequest)
			return
		}
		if err := service.HandleEvent(r.Context(), event); err != nil {
			if r.Context().Err() != nil {
				return
			}
			log.Printf("agent-events: %v", err)
			status := http.StatusServiceUnavailable
			var validation *agentbridge.ValidationError
			if errors.As(err, &validation) {
				status = http.StatusBadRequest
			}
			http.Error(w, err.Error(), status)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})
}
