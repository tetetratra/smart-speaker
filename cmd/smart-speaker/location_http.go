package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/tetetratra/smart-speaker/internal/states/userlocation"
)

const maxLocationRequestBytes = 4 * 1024

func registerLocationAPI(mux *http.ServeMux, store *userlocation.Store) {
	mux.HandleFunc("/api/location", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, store.Snapshot())
		case http.MethodPost:
			var req struct {
				Place string `json:"place"`
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxLocationRequestBytes)
			decoder := json.NewDecoder(r.Body)
			err := decoder.Decode(&req)
			if err == nil {
				// 本文を最後まで検証してから現在値を更新する。
				if trailingErr := decoder.Decode(new(any)); trailingErr != io.EOF {
					err = trailingErr
					if err == nil {
						err = errors.New("複数のJSON値は指定できません")
					}
				}
			}
			if err != nil {
				var sizeErr *http.MaxBytesError
				if errors.As(err, &sizeErr) {
					http.Error(w, "本文は4KiB以内で指定してください", http.StatusRequestEntityTooLarge)
				} else {
					http.Error(w, "有効なJSONオブジェクトを指定してください", http.StatusBadRequest)
				}
				return
			}
			if strings.TrimSpace(req.Place) == "" {
				http.Error(w, "placeには空でない文字列を指定してください", http.StatusBadRequest)
				return
			}
			writeJSON(w, http.StatusOK, store.Update(req.Place))
		default:
			w.Header().Set("Allow", "GET, POST")
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}
