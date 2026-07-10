package memory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestEmbeddingClientEmbedSendsTEIRequest(t *testing.T) {
	var method string
	var path string
	var contentType string
	var payload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		contentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[[0.1,0.2,0.3]]`))
	}))
	defer srv.Close()

	client, err := NewEmbeddingClient(EmbeddingClientConfig{
		BaseURL:    srv.URL + "/",
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("NewEmbeddingClient() error = %v", err)
	}
	got, err := client.Embed(context.Background(), "検索対象")
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if method != http.MethodPost {
		t.Fatalf("method = %q, want %q", method, http.MethodPost)
	}
	if path != "/embed" {
		t.Fatalf("path = %q, want /embed", path)
	}
	if contentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}
	if payload["inputs"] != "検索対象" {
		t.Fatalf("inputs = %#v", payload["inputs"])
	}
	if _, ok := payload["prompt_name"]; ok {
		t.Fatalf("prompt_name should be omitted: %#v", payload)
	}
	want := []float64{0.1, 0.2, 0.3}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("embedding = %#v, want %#v", got, want)
	}
}

func TestEmbeddingClientEmbedSendsPromptName(t *testing.T) {
	var payload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode request body: %v", err)
		}
		_, _ = w.Write([]byte(`[[0.1]]`))
	}))
	defer srv.Close()

	client, err := NewEmbeddingClient(EmbeddingClientConfig{
		BaseURL:    srv.URL,
		PromptName: "query",
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("NewEmbeddingClient() error = %v", err)
	}
	if _, err := client.Embed(context.Background(), "query text"); err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if payload["prompt_name"] != "query" {
		t.Fatalf("prompt_name = %#v, want query", payload["prompt_name"])
	}
}

func TestNewEmbeddingClientDoesNotConnect(t *testing.T) {
	client, err := NewEmbeddingClient(EmbeddingClientConfig{
		BaseURL: "http://127.0.0.1:1",
	})
	if err != nil {
		t.Fatalf("NewEmbeddingClient() error = %v", err)
	}
	if client == nil {
		t.Fatal("client is nil")
	}
}

func TestNewEmbeddingClientUsesDefaults(t *testing.T) {
	client, err := NewEmbeddingClient(EmbeddingClientConfig{})
	if err != nil {
		t.Fatalf("NewEmbeddingClient() error = %v", err)
	}
	if client.baseURL != DefaultEmbeddingBaseURL {
		t.Fatalf("baseURL = %q, want %q", client.baseURL, DefaultEmbeddingBaseURL)
	}
}

func TestNewEmbeddingClientRejectsInvalidBaseURL(t *testing.T) {
	_, err := NewEmbeddingClient(EmbeddingClientConfig{BaseURL: "://bad"})
	if err == nil || !strings.Contains(err.Error(), "base URL is invalid") {
		t.Fatalf("err = %v, want invalid base URL", err)
	}
}

func TestEmbeddingClientEmbedRejectsEmptyText(t *testing.T) {
	client, err := NewEmbeddingClient(EmbeddingClientConfig{})
	if err != nil {
		t.Fatalf("NewEmbeddingClient() error = %v", err)
	}
	_, err = client.Embed(context.Background(), "  ")
	if err == nil || !strings.Contains(err.Error(), "text is required") {
		t.Fatalf("err = %v, want text is required", err)
	}
}

func TestEmbeddingClientEmbedReturnsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer srv.Close()

	client, err := NewEmbeddingClient(EmbeddingClientConfig{
		BaseURL:    srv.URL,
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("NewEmbeddingClient() error = %v", err)
	}
	_, err = client.Embed(context.Background(), "text")
	if err == nil || !strings.Contains(err.Error(), "400 Bad Request") {
		t.Fatalf("err = %v, want status error", err)
	}
}

func TestEmbeddingClientEmbedRejectsEmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	client, err := NewEmbeddingClient(EmbeddingClientConfig{
		BaseURL:    srv.URL,
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("NewEmbeddingClient() error = %v", err)
	}
	_, err = client.Embed(context.Background(), "text")
	if err == nil || !strings.Contains(err.Error(), "response is empty") {
		t.Fatalf("err = %v, want empty response error", err)
	}
}

func TestEmbeddingClientEmbedRejectsEmptyEmbedding(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[[]]`))
	}))
	defer srv.Close()

	client, err := NewEmbeddingClient(EmbeddingClientConfig{
		BaseURL:    srv.URL,
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("NewEmbeddingClient() error = %v", err)
	}
	_, err = client.Embed(context.Background(), "text")
	if err == nil || !strings.Contains(err.Error(), "embedding is empty") {
		t.Fatalf("err = %v, want empty embedding error", err)
	}
}
