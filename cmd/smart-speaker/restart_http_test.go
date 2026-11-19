package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRestartAPI(t *testing.T) {
	restarted := make(chan struct{})
	mux := http.NewServeMux()
	registerRestartAPI(mux, func() { close(restarted) })

	// GET では再起動しない。
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/restart", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET: %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/restart", nil))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST: %d", rec.Code)
	}
	select {
	case <-restarted:
	case <-time.After(5 * time.Second):
		t.Fatal("再起動が呼ばれません")
	}
}

func TestRestartAPINotRegisteredWithoutRestart(t *testing.T) {
	mux := http.NewServeMux()
	registerRestartAPI(mux, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/restart", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("POST: %d", rec.Code)
	}
}
