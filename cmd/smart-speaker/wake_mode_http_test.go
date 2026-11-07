package main

import (
	"github.com/tetetratra/smart-speaker/internal/states/wakemode"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWakeModeAPIControlsPersistentSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mode.json")
	store, _ := wakemode.NewStore(path)
	mux := http.NewServeMux()
	registerWakeModeAPI(mux, store)
	request := func(method, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(method, "/api/wake-mode", strings.NewReader(body)))
		return rec
	}
	if rec := request("GET", ""); rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(rec.Code)
	}
	for _, body := range []string{`{}`, `{"required":"true"}`, `{"required":true} {}`, `{"required":true,"words":["！？"]}`, `{"required":true,"max_prefix":-1}`, `{"required":true,"patterns":["^起きて"]}`} {
		if rec := request("PUT", body); rec.Code != 400 {
			t.Fatal(rec.Code, rec.Body)
		}
	}
	if rec := request("PUT", `{"required":true,"words":["ねえ"],"max_prefix":2,"min_similarity":0.7}`); rec.Code != 200 {
		t.Fatal(rec.Code, rec.Body)
	}
	restored, err := wakemode.NewStore(path)
	if err != nil || !restored.Snapshot(time.Now()).Required || restored.Snapshot(time.Now()).Words[0] != "ねえ" || restored.Snapshot(time.Now()).MaxPrefix != 2 {
		t.Fatal(err, restored)
	}
	if rec := request("DELETE", ""); rec.Code != 405 {
		t.Fatal(rec.Code)
	}
}
