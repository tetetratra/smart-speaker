package main

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/tetetratra/smart-speaker/internal/components/tts"
)

func registerTTSAPI(mux *http.ServeMux, selector *tts.Selector) {
	mux.HandleFunc("/api/tts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodGet && r.Method != http.MethodPut {
			w.Header().Set("Allow", "GET, PUT")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if selector == nil {
			http.Error(w, "TTSを利用できません", http.StatusServiceUnavailable)
			return
		}
		if r.Method == http.MethodGet {
			writeJSON(w, http.StatusOK, selector.Snapshot())
			return
		}
		var req struct {
			Provider string `json:"provider"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			http.Error(w, "providerを含むJSONを指定してください", http.StatusBadRequest)
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			http.Error(w, "JSONは1件だけ指定してください", http.StatusBadRequest)
			return
		}
		selection, err := selector.Select(req.Provider)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, selection)
	})
}
