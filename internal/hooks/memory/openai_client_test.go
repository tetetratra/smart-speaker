package memory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	types "github.com/tetetratra/smart-speaker/internal/types"
)

func TestOpenAIClientCreateCandidatesSendsStructuredOutputSchema(t *testing.T) {
	var method string
	var contentType string
	var authorization string
	var payload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		contentType = r.Header.Get("Content-Type")
		authorization = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output_text":"{\"candidates\":[{\"content\":\"ユーザーは朝にコーヒーを飲む\",\"tags\":[\"routine\",\"coffee\"]}]}"}`))
	}))
	defer srv.Close()

	client, err := NewOpenAIClient(OpenAIClientConfig{
		APIKey:        "test-key",
		Model:         "test-model",
		Endpoint:      srv.URL,
		HTTPClient:    srv.Client(),
		MaxCandidates: 3,
		MaxTags:       4,
	})
	if err != nil {
		t.Fatalf("NewOpenAIClient() error = %v", err)
	}
	got, err := client.CreateCandidates(context.Background(), []types.ConversationRecord{
		{
			ID:           "rec-1",
			Role:         types.RoleUser,
			Text:         "朝はコーヒーが多い",
			GenerationID: 2,
			Source:       "test",
			Metadata:     map[string]any{"lang": "ja"},
			CreatedAt:    time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("CreateCandidates() error = %v", err)
	}
	if method != http.MethodPost {
		t.Fatalf("method = %q, want %q", method, http.MethodPost)
	}
	if contentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}
	if authorization != "Bearer test-key" {
		t.Fatalf("Authorization = %q", authorization)
	}
	if payload["model"] != "test-model" {
		t.Fatalf("model = %#v, want test-model", payload["model"])
	}
	input := payload["input"].([]any)
	if len(input) != 2 {
		t.Fatalf("input len = %d, want 2", len(input))
	}
	system := input[0].(map[string]any)
	instructions := system["content"].(string)
	for _, want := range []string{"出力例", "SwitchBot ハブミニ", "candidates: []"} {
		if !strings.Contains(instructions, want) {
			t.Fatalf("system instructions = %s, want it to contain %s", instructions, want)
		}
	}
	history := input[1].(map[string]any)
	if history["role"] != types.RoleUser {
		t.Fatalf("history role = %v, want user", history["role"])
	}
	content := history["content"].(string)
	for _, want := range []string{`"role":"user"`, `"text":"朝はコーヒーが多い"`, `"generation_id":2`, `"lang":"ja"`} {
		if !strings.Contains(content, want) {
			t.Fatalf("history content = %s, want it to contain %s", content, want)
		}
	}
	format := payload["text"].(map[string]any)["format"].(map[string]any)
	if format["type"] != "json_schema" || format["name"] != "memory_candidates" || format["strict"] != true {
		t.Fatalf("format = %#v", format)
	}
	schema, _ := json.Marshal(format["schema"])
	schemaText := string(schema)
	for _, want := range []string{`"candidates"`, `"content"`, `"tags"`, `"maxItems":3`, `"maxItems":4`, `"additionalProperties":false`} {
		if !strings.Contains(schemaText, want) {
			t.Fatalf("schema = %s, want it to contain %s", schemaText, want)
		}
	}
	if len(got) != 1 || got[0].Content != "ユーザーは朝にコーヒーを飲む" {
		t.Fatalf("candidates = %#v", got)
	}
}

func TestOpenAIClientCreateCandidatesReturnsEmptyWithoutHistory(t *testing.T) {
	client := &OpenAIClient{
		apiKey:        "test-key",
		model:         "test-model",
		endpoint:      "http://127.0.0.1:1",
		client:        &http.Client{},
		maxCandidates: DefaultMaxMemoryCandidates,
		maxTags:       DefaultMaxMemoryCandidateTags,
	}
	got, err := client.CreateCandidates(context.Background(), nil)
	if err != nil {
		t.Fatalf("CreateCandidates() error = %v", err)
	}
	if got != nil {
		t.Fatalf("candidates = %#v, want nil", got)
	}
}

func TestOpenAIClientCreateCandidatesReadsOutputContentText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"output": [
				{"type":"message","content":[{"type":"output_text","text":"{\"candidates\":[{\"content\":\"ユーザーはリビングの照明をSwitchBotで操作する\",\"tags\":[\"SwitchBot\",\"lighting\"]}]}"}]}
			]
		}`))
	}))
	defer srv.Close()

	client, err := NewOpenAIClient(OpenAIClientConfig{
		APIKey:     "test-key",
		Model:      "test-model",
		Endpoint:   srv.URL,
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("NewOpenAIClient() error = %v", err)
	}
	got, err := client.CreateCandidates(context.Background(), []types.ConversationRecord{{Role: types.RoleUser, Text: "照明はSwitchBot"}})
	if err != nil {
		t.Fatalf("CreateCandidates() error = %v", err)
	}
	if len(got) != 1 || got[0].Tags[0] != "SwitchBot" {
		t.Fatalf("candidates = %#v", got)
	}
}

func TestOpenAIClientCreateCandidatesNormalizesCandidates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"output_text":"{\"candidates\":[{\"content\":\"  朝はコーヒー  \",\"tags\":[\" coffee \",\"Coffee\",\"morning\",\"routine\"]},{\"content\":\"   \",\"tags\":[\"skip\"]},{\"content\":\"雨の日に頭痛が出やすい\",\"tags\":[\"health\",\"weather\"]}]}"}`))
	}))
	defer srv.Close()

	client, err := NewOpenAIClient(OpenAIClientConfig{
		APIKey:        "test-key",
		Model:         "test-model",
		Endpoint:      srv.URL,
		HTTPClient:    srv.Client(),
		MaxCandidates: 1,
		MaxTags:       2,
	})
	if err != nil {
		t.Fatalf("NewOpenAIClient() error = %v", err)
	}
	got, err := client.CreateCandidates(context.Background(), []types.ConversationRecord{{Role: types.RoleUser, Text: "test"}})
	if err != nil {
		t.Fatalf("CreateCandidates() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("candidates len = %d, want 1", len(got))
	}
	if got[0].Content != "朝はコーヒー" {
		t.Fatalf("Content = %q, want trimmed", got[0].Content)
	}
	wantTags := []string{"coffee", "morning"}
	if !sameStringSlice(got[0].Tags, wantTags) {
		t.Fatalf("Tags = %#v, want %#v", got[0].Tags, wantTags)
	}
}

func TestOpenAIClientCreateCandidatesReturnsErrors(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		body     string
		wantText string
	}{
		{name: "http error", status: http.StatusBadRequest, body: "bad request", wantText: "400 Bad Request"},
		{name: "response error", status: http.StatusOK, body: `{"error":{"message":"failed"}}`, wantText: "response failed: failed"},
		{name: "empty text", status: http.StatusOK, body: `{"output":[]}`, wantText: "response text is empty"},
		{name: "parse error", status: http.StatusOK, body: `{"output_text":"not json"}`, wantText: "parse candidates"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			client, err := NewOpenAIClient(OpenAIClientConfig{
				APIKey:     "test-key",
				Model:      "test-model",
				Endpoint:   srv.URL,
				HTTPClient: srv.Client(),
			})
			if err != nil {
				t.Fatalf("NewOpenAIClient() error = %v", err)
			}
			_, err = client.CreateCandidates(context.Background(), []types.ConversationRecord{{Role: types.RoleUser, Text: "test"}})
			if err == nil || !strings.Contains(err.Error(), tt.wantText) {
				t.Fatalf("err = %v, want it to contain %q", err, tt.wantText)
			}
		})
	}
}

func TestNewOpenAIClientValidatesConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  OpenAIClientConfig
		want string
	}{
		{name: "api key", cfg: OpenAIClientConfig{Model: "model"}, want: "API key is required"},
		{name: "model", cfg: OpenAIClientConfig{APIKey: "key"}, want: "model is required"},
		{name: "endpoint", cfg: OpenAIClientConfig{APIKey: "key", Model: "model", Endpoint: "://bad"}, want: "endpoint is invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewOpenAIClient(tt.cfg)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func sameStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
