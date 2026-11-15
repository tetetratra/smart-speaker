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

func TestJobsAPIRegistersUpdatesAndDeletes(t *testing.T) {
	service, _ := automations.New(filepath.Join(t.TempDir(), "jobs.json"), nil)
	mux := http.NewServeMux()
	registerAutomationsAPI(mux, service)
	request := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		return rec
	}
	rec := request("POST", "/api/jobs", `{"name":"仕事","script":"toolcall(\"set_wake_word_required\", {\"required\":true})","schedule":{"cron":"0 8 * * 1-5","timezone":"Asia/Tokyo"}}`)
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
	if rec = request("PATCH", "/api/jobs/"+id, `{"enabled":false}`); rec.Code != 405 {
		t.Fatal("停止・再開は提供しません", rec.Code)
	}
	if rec = request("PUT", "/api/jobs/"+id, `{"name":"変更","script":"remind(\"変更後の指示\")","schedule":{"after_seconds":600}}`); rec.Code != 200 || service.List()[0].Script != `remind("変更後の指示")` {
		t.Fatal(rec.Code, rec.Body)
	}
	if rec = request("POST", "/api/jobs", `{"name":"不正な構文","script":"function (","schedule":{"after_seconds":10}}`); rec.Code != 400 {
		t.Fatal("構文不正のJSを受理しました", rec.Code)
	}
	if rec = request("DELETE", "/api/jobs/"+id, ""); rec.Code != 204 {
		t.Fatal(rec.Code)
	}
	if rec = request("DELETE", "/api/jobs/"+id, ""); rec.Code != 404 {
		t.Fatal(rec.Code)
	}
}

func TestJobsAPIListsEvents(t *testing.T) {
	service, _ := automations.New(filepath.Join(t.TempDir(), "jobs.json"), nil)
	service.SetEvents(map[string]string{"EventB": "Bの説明", "EventA": "Aの説明"})
	mux := http.NewServeMux()
	registerAutomationsAPI(mux, service)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/jobs", nil))
	var data struct {
		Events []automationEvent `json:"events"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	want := []automationEvent{{"EventA", "Aの説明"}, {"EventB", "Bの説明"}}
	if len(data.Events) != len(want) || data.Events[0] != want[0] || data.Events[1] != want[1] {
		t.Fatalf("events = %+v, want %+v", data.Events, want)
	}
}
