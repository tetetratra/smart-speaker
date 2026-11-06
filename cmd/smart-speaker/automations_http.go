package main

import (
	"encoding/json"
	"errors"
	"github.com/tetetratra/smart-speaker/internal/automations"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func registerAutomationsAPI(mux *http.ServeMux, service *automations.Service) {
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
			writeJSON(w, 200, map[string]any{"jobs": service.List()})
		case collection && r.Method == http.MethodPost:
			var spec automations.Spec
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
			var spec automations.Spec
			if !decode(&spec) {
				return
			}
			job, err := service.Update(id, spec, time.Now())
			if err != nil {
				fail(err)
				return
			}
			writeJSON(w, 200, map[string]any{"job": job})
		case !collection && r.Method == http.MethodPatch:
			var req struct {
				Enabled *bool `json:"enabled"`
			}
			if !decode(&req) {
				return
			}
			if req.Enabled == nil {
				http.Error(w, "enabledを指定してください", 400)
				return
			}
			job, err := service.Enable(id, *req.Enabled, time.Now())
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
				w.Header().Set("Allow", "PUT, PATCH, DELETE")
			}
			w.WriteHeader(405)
		}
	}
	mux.HandleFunc("/api/jobs", handler)
	mux.HandleFunc("/api/jobs/", handler)
}
