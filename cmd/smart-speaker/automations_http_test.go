package main

import (
	"encoding/json"
	"github.com/tetetratra/smart-speaker/internal/automations"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestJobsAPIRegistersUpdatesStopsAndDeletes(t *testing.T) {
	service, _ := automations.New(filepath.Join(t.TempDir(), "jobs.json"), nil)
	mux := http.NewServeMux()
	registerAutomationsAPI(mux, service)
	request := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		return rec
	}
	rec := request("POST", "/api/jobs", `{"name":"仕事","instruction":"モードをONにして","speak":false,"schedule":{"cron":"0 8 * * 1-5","timezone":"Asia/Tokyo"}}`)
	if rec.Code != 201 {
		t.Fatal(rec.Code, rec.Body)
	}
	var data struct {
		Job automations.Job `json:"job"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	id := data.Job.ID
	if rec = request("GET", "/api/jobs", ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), id) || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(rec.Code, rec.Body)
	}
	if rec = request("PATCH", "/api/jobs/"+id, `{"enabled":false}`); rec.Code != 200 || service.List()[0].Enabled {
		t.Fatal(rec.Code, rec.Body)
	}
	if rec = request("PUT", "/api/jobs/"+id, `{"name":"変更","instruction":"変更後の指示","speak":true,"schedule":{"after_seconds":600}}`); rec.Code != 200 || !service.List()[0].Speak {
		t.Fatal(rec.Code, rec.Body)
	}
	if rec = request("POST", "/api/jobs", `{"name":"コードは禁止","script":"bad()","schedule":{"after_seconds":10}}`); rec.Code != 400 {
		t.Fatal("コードを受理しました", rec.Code)
	}
	if rec = request("DELETE", "/api/jobs/"+id, ""); rec.Code != 204 {
		t.Fatal(rec.Code)
	}
	if rec = request("DELETE", "/api/jobs/"+id, ""); rec.Code != 404 {
		t.Fatal(rec.Code)
	}
}
