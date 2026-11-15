package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tetetratra/smart-speaker/internal/app"
	"github.com/tetetratra/smart-speaker/internal/states/userlocation"
	"github.com/tetetratra/smart-speaker/internal/types"
)

func TestLocationAPIThroughHTTPServer(t *testing.T) {
	newServer := func() *http.Server {
		cfg := app.Config{APIKey: "test-key", WebDistDir: t.TempDir()}
		cfg.Memory.Model = "test-model"
		services := runtimeServices{UserLocation: &userlocation.Store{}}
		server, chat, err := buildHTTPServer(cfg, nil, services)
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
		before := time.Now()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/location", strings.NewReader(string(body)+" \n"))
		req.Header.Set("Content-Type", "application/json")
		server.Handler.ServeHTTP(rec, req)
		after := time.Now()
		if rec.Code != http.StatusOK {
			t.Fatalf("POSTの応答が不正: status=%d body=%s", rec.Code, rec.Body.String())
		}
		var got userlocation.Record
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		// 更新日時はサーバーのタイムゾーン（本番ではAsia/Tokyo）の時差付きで返す。
		_, gotOffset := got.UpdatedAt.Zone()
		_, wantOffset := before.Zone()
		if got.Place != place || got.UpdatedAt.Before(before) || got.UpdatedAt.After(after) || gotOffset != wantOffset {
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
			initial, _ := store.Update("home")
			mux := http.NewServeMux()
			registerLocationAPI(mux, &store, nil)
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
	initial, _ := store.Update("home")
	mux := http.NewServeMux()
	registerLocationAPI(mux, &store, nil)
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

func TestLocationToolReadsHTTPUpdates(t *testing.T) {
	cfg := app.Config{WebDistDir: t.TempDir()}
	services := runtimeServices{UserLocation: &userlocation.Store{}}
	definitions, handlers := buildToolRegistry(cfg, nil, services)
	found := false
	for _, definition := range definitions {
		if definition.(map[string]any)["name"] == "get_current_location" {
			found = true
		}
	}
	if !found {
		t.Fatal("現在地ツールの定義がありません")
	}
	handler := handlers["get_current_location"]
	if handler == nil {
		t.Fatal("現在地ツールが登録されていません")
	}
	server, chat, err := buildHTTPServer(cfg, nil, services)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeHTTPServer(server); closeStages(chat) })
	check := func(want string) {
		t.Helper()
		result, err := handler.Run(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		if string(raw) != want {
			t.Fatalf("取得結果 = %s、期待値 = %s", raw, want)
		}
	}
	check(`{"location":null}`)
	for _, place := range []string{"自宅", "会社"} {
		rec := httptest.NewRecorder()
		server.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/location", strings.NewReader(`{"place":"`+place+`"}`)))
		if rec.Code != http.StatusOK {
			t.Fatalf("保存失敗: %s", rec.Body.String())
		}
		check(`{"location":` + strings.TrimSpace(rec.Body.String()) + `}`)
	}
}

// 場所が変わったときだけ、購読可能な記録として会話へ届ける。起動後の最初の1件は前の場所がないので変化として扱う。
func TestLocationAPIDeliversOnlyChanges(t *testing.T) {
	var store userlocation.Store
	var got []types.ConversationInput
	mux := http.NewServeMux()
	registerLocationAPI(mux, &store, func(_ context.Context, input types.ConversationInput) error {
		got = append(got, input)
		return nil
	})
	for _, place := range []string{"自宅", "自宅", "会社", "会社", "自宅"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/location", strings.NewReader(`{"place":"`+place+`"}`)))
		if rec.Code != http.StatusOK {
			t.Fatalf("保存失敗: %s", rec.Body.String())
		}
	}
	want := []struct {
		previous any
		place    string
		text     string
	}{
		{nil, "自宅", "ユーザーの居場所が「自宅」になりました。"},
		{"自宅", "会社", "ユーザーの居場所が「自宅」から「会社」に変わりました。"},
		{"会社", "自宅", "ユーザーの居場所が「会社」から「自宅」に変わりました。"},
	}
	if len(got) != len(want) {
		t.Fatalf("届いた件数 = %d、期待値 = %d: %#v", len(got), len(want), got)
	}
	for i, input := range got {
		r := input.Record
		if !input.Subscribable || input.ModelShouldRespond || input.ModelAlreadyKnows || !input.UserAlreadyKnows {
			t.Fatalf("%d件目のフラグが不正: %#v", i, input)
		}
		if r.Source != userLocationSource || r.Role != types.RoleSystem || r.Text != want[i].text || r.CreatedAt.IsZero() ||
			r.Metadata["place"] != want[i].place || r.Metadata["previous_place"] != want[i].previous {
			t.Fatalf("%d件目の記録が不正: %#v", i, r)
		}
	}
}

// 位置情報APIが送る記録の Source は、予約ジョブで指定できる一覧に載っている。
func TestLocationSourceIsSubscribable(t *testing.T) {
	if _, ok := automationSources[userLocationSource]; !ok {
		t.Fatalf("%s が automationSources にありません", userLocationSource)
	}
}
