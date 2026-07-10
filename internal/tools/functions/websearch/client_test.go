package websearch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
		_, _ = w.Write([]byte(`{"output_text":"回答本文"}`))
	}))
	defer srv.Close()

	client := NewClient(ClientConfig{
		APIKey:     "test-key",
		Model:      "test-model",
		HTTPClient: srv.Client(),
		Endpoint:   srv.URL,
	})
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
	_, err := readSearchResponse(strings.NewReader(`{"output":[]}`))
	if err == nil || !strings.Contains(err.Error(), "result is empty") {
		t.Fatalf("err = %v, want result is empty", err)
	}
}
