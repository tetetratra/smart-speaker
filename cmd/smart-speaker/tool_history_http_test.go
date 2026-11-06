package main

import (
	"encoding/json"
	"github.com/tetetratra/smart-speaker/internal/states/toolhistory"
	"github.com/tetetratra/smart-speaker/internal/types"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestToolHistoryAPIKeepsFullArgumentsAndResultAcrossReloads(t *testing.T) {
	store := toolhistory.NewStore(200)
	long := strings.Repeat("長い日本語", 3000)
	args, _ := json.Marshal(map[string]string{"query": long})
	output, _ := json.Marshal(map[string]string{"result": long})
	id := store.Begin(types.ToolRequest{ToolCallID: "call-1", Name: "web_search", Arguments: args})
	store.Finish(id, types.ToolResultRecord{Output: output})
	mux := http.NewServeMux()
	registerToolHistoryAPI(mux, store)
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/tool-history", nil))
		if rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(rec.Code, rec.Header())
		}
		var data struct {
			Entries []toolhistory.Entry `json:"entries"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &data); err != nil {
			t.Fatal(err)
		}
		if len(data.Entries) != 1 || data.Entries[0].Arguments != string(args) || data.Entries[0].Output != string(output) {
			t.Fatal("引数または結果が欠落しました")
		}
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/tool-history", nil))
	if rec.Code != 405 || rec.Header().Get("Allow") != "GET" {
		t.Fatal(rec.Code)
	}
}
