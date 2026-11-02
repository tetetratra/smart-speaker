package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tetetratra/smart-speaker/internal/app"
	"github.com/tetetratra/smart-speaker/internal/states/userlocation"
)

func TestLocationAPIThroughHTTPServer(t *testing.T) {
	newServer := func() *http.Server {
		cfg := app.Config{APIKey: "test-key", WebDistDir: t.TempDir()}
		cfg.Memory.Model = "test-model"
		server, chat, err := buildHTTPServer(cfg, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			closeHTTPServer(server)
			closeStages(chat)
		})
		return server
	}
	server := newServer()
	get := func(server *http.Server) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		server.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/location", nil))
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/json" || rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("GETの応答が不正: status=%d header=%v body=%s", rec.Code, rec.Header(), rec.Body.String())
		}
		return rec
	}
	if got := strings.TrimSpace(get(server).Body.String()); got != "null" {
		t.Fatalf("初期値 = %s、期待値はnull", got)
	}
	for _, place := range []string{"home", "会社", "会社", "  外出先  "} {
		body, err := json.Marshal(map[string]string{"place": place})
		if err != nil {
			t.Fatal(err)
		}
		before := time.Now().UTC()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/location", strings.NewReader(string(body)+" \n"))
		req.Header.Set("Content-Type", "application/json")
		server.Handler.ServeHTTP(rec, req)
		after := time.Now().UTC()
		if rec.Code != http.StatusOK {
			t.Fatalf("POSTの応答が不正: status=%d body=%s", rec.Code, rec.Body.String())
		}
		var got userlocation.Record
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Place != place || got.UpdatedAt.Before(before) || got.UpdatedAt.After(after) || !strings.Contains(rec.Body.String(), "Z\"") {
			t.Fatalf("POSTの保存値が不正: %#v", got)
		}
		if fetched := get(server).Body.String(); fetched != rec.Body.String() {
			t.Fatalf("GETがPOSTの結果と異なる: GET=%s POST=%s", fetched, rec.Body.String())
		}
	}
	if got := strings.TrimSpace(get(newServer()).Body.String()); got != "null" {
		t.Fatalf("サーバー再構築後の値 = %s、期待値はnull", got)
	}
}

func TestLocationAPIRejectsInvalidRequestsWithoutChangingState(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		status int
	}{
		{"本文なし", "", http.StatusBadRequest},
		{"欠落", `{}`, http.StatusBadRequest},
		{"空文字", `{"place":""}`, http.StatusBadRequest},
		{"空白のみ", `{"place":" \t　"}`, http.StatusBadRequest},
		{"nullの場所", `{"place":null}`, http.StatusBadRequest},
		{"数値", `{"place":123}`, http.StatusBadRequest},
		{"配列の場所", `{"place":["home"]}`, http.StatusBadRequest},
		{"オブジェクトの場所", `{"place":{}}`, http.StatusBadRequest},
		{"壊れたJSON", `{"place":"home"`, http.StatusBadRequest},
		{"配列", `[]`, http.StatusBadRequest},
		{"nullの本文", `null`, http.StatusBadRequest},
		{"複数のJSON値", `{"place":"office"} {"place":"home"}`, http.StatusBadRequest},
		{"末尾null", `{"place":"office"} null`, http.StatusBadRequest},
		{"末尾の不正値", `{"place":"office"} garbage`, http.StatusBadRequest},
		{"本文サイズ超過", `{"place":"` + strings.Repeat("x", maxLocationRequestBytes) + `"}`, http.StatusRequestEntityTooLarge},
		{"末尾空白でサイズ超過", `{"place":"office"}` + strings.Repeat(" ", maxLocationRequestBytes), http.StatusRequestEntityTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var store userlocation.Store
			initial := store.Update("home")
			mux := http.NewServeMux()
			registerLocationAPI(mux, &store)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/location", strings.NewReader(tt.body)))
			if rec.Code != tt.status {
				t.Fatalf("status = %d、期待値 = %d、本文 = %s", rec.Code, tt.status, rec.Body.String())
			}
			if got := store.Snapshot(); got == nil || *got != initial {
				t.Fatalf("不正入力で現在地が変更された: %#v", got)
			}
		})
	}
}

func TestLocationAPIRejectsUnsupportedMethods(t *testing.T) {
	var store userlocation.Store
	initial := store.Update("home")
	mux := http.NewServeMux()
	registerLocationAPI(mux, &store)
	for _, method := range []string{http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions} {
		t.Run(method, func(t *testing.T) {
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(method, "/api/location", strings.NewReader(`{"place":"office"}`)))
			if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != "GET, POST" {
				t.Fatalf("非対応メソッドの応答が不正: status=%d header=%v", rec.Code, rec.Header())
			}
			if got := store.Snapshot(); got == nil || *got != initial {
				t.Fatalf("非対応メソッドで現在地が変更された: %#v", got)
			}
		})
	}
}
