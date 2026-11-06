package main

import (
	"github.com/tetetratra/smart-speaker/internal/states/toolhistory"
	"net/http"
)

func registerToolHistoryAPI(mux *http.ServeMux, store *toolhistory.Store) {
	mux.HandleFunc("/api/tool-history", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if store == nil {
			http.Error(w, "ツール履歴を利用できません", http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"entries": store.Snapshot(), "limit": store.Limit(), "persistence": "memory"})
	})
}
