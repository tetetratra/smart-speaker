package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/tetetratra/smart-speaker/internal/subagent"
)

func registerSubagentAPI(mux *http.ServeMux, service *subagent.Service) {
	mux.HandleFunc("/api/subagent/permissions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if service == nil {
			writeJSON(w, 200, map[string]any{"requests": []any{}})
			return
		}
		writeJSON(w, 200, map[string]any{"requests": service.Pending()})
	})
	mux.HandleFunc("/api/subagent/permissions/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if service == nil {
			http.Error(w, "サブエージェントを利用できません", 503)
			return
		}
		var req struct {
			Approved *bool `json:"approved"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil || req.Approved == nil {
			http.Error(w, "approvedにtrueまたはfalseを指定してください", 400)
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			http.Error(w, "JSONは1件だけ指定してください", 400)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/subagent/permissions/")
		if err := service.Resolve(id, *req.Approved); err != nil {
			http.Error(w, err.Error(), 409)
			return
		}
		writeJSON(w, 200, map[string]any{"accepted": true})
	})
}
