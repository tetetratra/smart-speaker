package automations

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

// RegisterHTTP は、画面から予約ジョブを一覧・追加・変更・削除する API を mux に登録する。
func RegisterHTTP(mux *http.ServeMux, service *Service) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if service == nil {
			http.Error(w, "予約機能を利用できません", 503)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/jobs/")
		collection := r.URL.Path == "/api/jobs"
		decode := func(value any) bool {
			d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536))
			d.DisallowUnknownFields()
			if err := d.Decode(value); err != nil {
				http.Error(w, "有効なJSONを指定してください", 400)
				return false
			}
			if err := d.Decode(new(any)); err != io.EOF {
				http.Error(w, "JSONは1件だけ指定してください", 400)
				return false
			}
			return true
		}
		fail := func(err error) {
			status := 400
			if errors.Is(err, os.ErrNotExist) {
				status = 404
			}
			http.Error(w, err.Error(), status)
		}
		switch {
		case collection && r.Method == http.MethodGet:
			writeJSON(w, 200, map[string]any{"jobs": service.List(), "sources": automationSourceList(service)})
		case collection && r.Method == http.MethodPost:
			var spec Spec
			if !decode(&spec) {
				return
			}
			job, err := service.Create(spec, time.Now())
			if err != nil {
				fail(err)
				return
			}
			writeJSON(w, 201, map[string]any{"job": job})
		case !collection && r.Method == http.MethodPut:
			var spec Spec
			if !decode(&spec) {
				return
			}
			job, err := service.Update(id, spec, time.Now())
			if err != nil {
				fail(err)
				return
			}
			writeJSON(w, 200, map[string]any{"job": job})
		case !collection && r.Method == http.MethodDelete:
			if err := service.Delete(id); err != nil {
				fail(err)
				return
			}
			w.WriteHeader(204)
		default:
			if collection {
				w.Header().Set("Allow", "GET, POST")
			} else {
				w.Header().Set("Allow", "PUT, DELETE")
			}
			w.WriteHeader(405)
		}
	}
	mux.HandleFunc("/api/jobs", handler)
	mux.HandleFunc("/api/jobs/", handler)
}

// automationSource は、画面の予約の編集で schedule.source に選べる記録の Source。
type automationSource struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// automationSourceList は、schedule.source に指定できる記録の Source を名前順に並べて返す。
func automationSourceList(service *Service) []automationSource {
	sources := service.Sources()
	list := make([]automationSource, 0, len(sources))
	for name, description := range sources {
		list = append(list, automationSource{Name: name, Description: description})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	return list
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("http: encode response: %v", err)
	}
}
