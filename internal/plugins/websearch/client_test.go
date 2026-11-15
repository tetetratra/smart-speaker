package websearch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientSearchSendsHostedWebSearchRequest(t *testing.T) {
	var payload map[string]any
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"completed","output_text":"回答本文","output":[{"type":"web_search_call","status":"completed"}]}`))
	}))
	defer srv.Close()

	client := NewClient(ClientConfig{
		APIKey:     "test-key",
		Model:      "test-model",
		HTTPClient: srv.Client(),
		Endpoint:   srv.URL,
	})
	before := time.Now()
	got, err := client.Search(context.Background(), "今日のニュース")
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if got != "回答本文" {
		t.Fatalf("result = %q", got)
	}
	if auth != "Bearer test-key" {
		t.Fatalf("Authorization = %q", auth)
	}
	if payload["model"] != "test-model" {
		t.Fatalf("model = %#v", payload["model"])
	}
	if payload["input"] != "検索依頼:\n今日のニュース" {
		t.Fatalf("input = %#v", payload["input"])
	}
	instructions, ok := payload["instructions"].(string)
	if !ok || !strings.Contains(instructions, "逆質問") {
		t.Fatalf("instructions = %#v", payload["instructions"])
	}
	_, timestamp, ok := strings.Cut(instructions, "現在日時: ")
	if !ok {
		t.Fatal("検索に現在日時が渡されていません")
	}
	timestamp, _, _ = strings.Cut(timestamp, "\n")
	now, err := time.Parse(time.RFC3339, timestamp)
	_, offset := now.Zone()
	if err != nil || offset != 9*60*60 || now.Before(before.Truncate(time.Second)) || now.After(time.Now()) {
		t.Fatalf("検索の日時が日本時間の現在日時ではありません: %s, %v", timestamp, err)
	}
	if !strings.Contains(instructions, "今日: "+now.Format(time.DateOnly)) {
		t.Fatalf("検索の対象日がありません: %s", instructions)
	}
	if payload["tool_choice"] != "required" {
		t.Fatalf("Web検索が必須になっていません: %#v", payload["tool_choice"])
	}
	tools, ok := payload["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools = %#v", payload["tools"])
	}
	tool, ok := tools[0].(map[string]any)
	if !ok || tool["type"] != "web_search" {
		t.Fatalf("tool = %#v", tools[0])
	}
	encodedInclude, _ := json.Marshal(payload["include"])
	if !strings.Contains(string(encodedInclude), "web_search_call.action.sources") {
		t.Fatalf("include = %s", encodedInclude)
	}
}

func TestClientSearchReturnsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer srv.Close()

	client := NewClient(ClientConfig{
		APIKey:     "test-key",
		Model:      "test-model",
		HTTPClient: srv.Client(),
		Endpoint:   srv.URL,
	})
	_, err := client.Search(context.Background(), "query")
	if err == nil || !strings.Contains(err.Error(), "400 Bad Request") {
		t.Fatalf("err = %v, want status error", err)
	}
}

func TestReadSearchResponseFallsBackToMessageContent(t *testing.T) {
	raw := `{
		"output": [
			{"type": "web_search_call", "status": "completed"},
			{
				"type": "message",
				"content": [
					{"type": "output_text", "text": "fallback result"}
				]
			}
		]
	}`
	got, err := readSearchResponse(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if got != "fallback result" {
		t.Fatalf("result = %q", got)
	}
}

func TestReadSearchResponseFailsWhenEmpty(t *testing.T) {
	_, err := readSearchResponse(strings.NewReader(`{"output":[{"type":"web_search_call","status":"completed"}]}`))
	if err == nil || !strings.Contains(err.Error(), "result is empty") {
		t.Fatalf("err = %v, want result is empty", err)
	}
}

func TestReadSearchResponseRejectsUnverifiedOrIncompleteSearch(t *testing.T) {
	for _, tc := range []struct{ name, raw string }{
		{"検索なしの本文", `{"output_text":"今日の東京は晴れです","output":[]}`},
		{"別のツールだけ完了", `{"output_text":"晴れ","output":[{"type":"function_call","status":"completed"}]}`},
		{"検索失敗", `{"output_text":"晴れ","output":[{"type":"web_search_call","status":"failed"}]}`},
		{"検索未完了", `{"output_text":"晴れ","output":[{"type":"web_search_call","status":"in_progress"}]}`},
		{"本文が途中", `{"status":"incomplete","output_text":"晴れ","output":[{"type":"web_search_call","status":"completed"}]}`},
		{"APIエラー", `{"error":{"message":"検索失敗"},"output_text":"晴れ","output":[{"type":"web_search_call","status":"completed"}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := readSearchResponse(strings.NewReader(tc.raw))
			if err == nil || got != "" {
				t.Fatalf("未確認の検索結果を採用しました: %q, %v", got, err)
			}
		})
	}
}
