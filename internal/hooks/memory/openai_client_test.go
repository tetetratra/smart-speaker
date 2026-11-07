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

func TestOpenAIClientCreateOperationsSendsStructuredOutputSchema(t *testing.T) {
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
		_, _ = w.Write([]byte(`{"output_text":"{\"operations\":[{\"action\":\"add\",\"target_id\":\"\",\"content\":\"ユーザーは朝にコーヒーを飲む\",\"tags\":[\"routine\",\"coffee\"]}]}"}`))
	}))
	defer srv.Close()

	client, err := NewOpenAIClient(OpenAIClientConfig{
		APIKey:        "test-key",
		Model:         "test-model",
		Endpoint:      srv.URL,
		HTTPClient:    srv.Client(),
		MaxOperations: 3,
		MaxTags:       4,
	})
	if err != nil {
		t.Fatalf("NewOpenAIClient() error = %v", err)
	}
	got, err := client.CreateOperations(context.Background(), []types.ConversationRecord{
		{
			ID:           "rec-1",
			Role:         types.RoleUser,
			Text:         "朝はコーヒーが多い",
			GenerationID: 2,
			Source:       "test",
			Metadata:     map[string]any{"lang": "ja"},
			CreatedAt:    time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC),
		},
	}, []ExistingMemory{{ID: "m-1", Content: "ユーザーは大岡山に住んでいる"}})
	if err != nil {
		t.Fatalf("CreateOperations() error = %v", err)
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
	for _, want := range []string{"出力例", "existing_memories", "迷ったら保存しない", "operations: []"} {
		if !strings.Contains(instructions, want) {
			t.Fatalf("system instructions = %s, want it to contain %s", instructions, want)
		}
	}
	history := input[1].(map[string]any)
	if history["role"] != types.RoleUser {
		t.Fatalf("history role = %v, want user", history["role"])
	}
	content := history["content"].(string)
	for _, want := range []string{`"role":"user"`, `"text":"朝はコーヒーが多い"`, `"existing_memories":[{"id":"m-1","content":"ユーザーは大岡山に住んでいる"}]`} {
		if !strings.Contains(content, want) {
			t.Fatalf("history content = %s, want it to contain %s", content, want)
		}
	}
	for _, unwanted := range []string{`generation_id`, `"lang":"ja"`, `rec-1`} {
		if strings.Contains(content, unwanted) {
			t.Fatalf("history content = %s, want it not to contain %s", content, unwanted)
		}
	}
	format := payload["text"].(map[string]any)["format"].(map[string]any)
	if format["type"] != "json_schema" || format["name"] != "memory_operations" || format["strict"] != true {
		t.Fatalf("format = %#v", format)
	}
	schema, _ := json.Marshal(format["schema"])
	schemaText := string(schema)
	for _, want := range []string{`"operations"`, `"action"`, `"target_id"`, `"content"`, `"tags"`, `"enum":["add","update","delete"]`, `"maxItems":3`, `"maxItems":4`, `"additionalProperties":false`} {
		if !strings.Contains(schemaText, want) {
			t.Fatalf("schema = %s, want it to contain %s", schemaText, want)
		}
	}
	if len(got) != 1 || got[0].Content != "ユーザーは朝にコーヒーを飲む" || got[0].Action != OperationAdd {
		t.Fatalf("operations = %#v", got)
	}
}

func TestOpenAIClientCreateOperationsSendsOnlyConversationTurns(t *testing.T) {
	var payload map[string]any
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode request body: %v", err)
		}
		_, _ = w.Write([]byte(`{"output_text":"{\"operations\":[]}"}`))
	}))
	defer srv.Close()

	client, err := NewOpenAIClient(OpenAIClientConfig{APIKey: "test-key", Model: "test-model", Endpoint: srv.URL, HTTPClient: srv.Client()})
	if err != nil {
		t.Fatalf("NewOpenAIClient() error = %v", err)
	}
	_, err = client.CreateOperations(context.Background(), []types.ConversationRecord{
		{Role: types.RoleUser, Text: "予定を教えて"},
		{Role: types.RoleToolCall, Text: `{"name":"calendar_list"}`},
		{Role: types.RoleToolResult, Text: `{"calendar":"primary","timezone":"Asia/Tokyo"}`},
		{Role: types.RoleSystem, Text: "(system: ユーザーに状況を尋ねてください)"},
		{Role: types.RoleAgent, Text: "今日は予定がありません"},
	}, nil)
	if err != nil {
		t.Fatalf("CreateOperations() error = %v", err)
	}
	if !called {
		t.Fatal("OpenAI was not called")
	}
	content := payload["input"].([]any)[1].(map[string]any)["content"].(string)
	var decoded operationInput
	if err := json.Unmarshal([]byte(content), &decoded); err != nil {
		t.Fatalf("Unmarshal input: %v", err)
	}
	if decoded.ExistingMemories == nil || len(decoded.ExistingMemories) != 0 {
		t.Fatalf("existing_memories = %#v, want empty array", decoded.ExistingMemories)
	}
	want := []conversationTurn{{Role: types.RoleUser, Text: "予定を教えて"}, {Role: types.RoleAgent, Text: "今日は予定がありません"}}
	if len(decoded.Conversation) != len(want) || decoded.Conversation[0] != want[0] || decoded.Conversation[1] != want[1] {
		t.Fatalf("conversation = %#v, want %#v", decoded.Conversation, want)
	}
}

func TestConversationTurnsFormatsCreatedAtInJapanTime(t *testing.T) {
	turns := conversationTurns([]types.ConversationRecord{{Role: types.RoleUser, Text: "おはよう", CreatedAt: time.Date(2026, 11, 6, 23, 30, 0, 0, time.UTC)}})
	if len(turns) != 1 || turns[0].CreatedAt != "2026-11-07T08:30:00+09:00" {
		t.Fatalf("turns = %#v, want created_at in +09:00", turns)
	}
}

func TestOpenAIClientCreateOperationsSkipsWithoutConversationTurns(t *testing.T) {
	client := &OpenAIClient{apiKey: "test-key", model: "test-model", endpoint: "http://127.0.0.1:1", client: &http.Client{}}
	got, err := client.CreateOperations(context.Background(), []types.ConversationRecord{
		{Role: types.RoleToolResult, Text: `{"ok":true}`},
		{Role: types.RoleUser, Text: "   "},
	}, nil)
	if err != nil {
		t.Fatalf("CreateOperations() error = %v", err)
	}
	if got != nil {
		t.Fatalf("operations = %#v, want nil", got)
	}
}

func TestOpenAIClientCreateOperationsReturnsEmptyWithoutHistory(t *testing.T) {
	client := &OpenAIClient{
		apiKey:        "test-key",
		model:         "test-model",
		endpoint:      "http://127.0.0.1:1",
		client:        &http.Client{},
		maxOperations: DefaultMaxMemoryOperations,
		maxTags:       DefaultMaxMemoryTags,
	}
	got, err := client.CreateOperations(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("CreateOperations() error = %v", err)
	}
	if got != nil {
		t.Fatalf("operations = %#v, want nil", got)
	}
}

func TestOpenAIClientCreateOperationsReadsOutputContentText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"output": [
				{"type":"message","content":[{"type":"output_text","text":"{\"operations\":[{\"action\":\"add\",\"target_id\":\"\",\"content\":\"ユーザーはリビングの照明をSwitchBotで操作する\",\"tags\":[\"SwitchBot\",\"lighting\"]}]}"}]}
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
	got, err := client.CreateOperations(context.Background(), []types.ConversationRecord{{Role: types.RoleUser, Text: "照明はSwitchBot"}}, nil)
	if err != nil {
		t.Fatalf("CreateOperations() error = %v", err)
	}
	if len(got) != 1 || got[0].Tags[0] != "SwitchBot" {
		t.Fatalf("operations = %#v", got)
	}
}

func TestOpenAIClientCreateOperationsNormalizesOperations(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"output_text":"{\"operations\":[{\"action\":\"add\",\"target_id\":\"\",\"content\":\"  朝はコーヒー  \",\"tags\":[\" coffee \",\"Coffee\",\"morning\",\"routine\"]},{\"action\":\"add\",\"target_id\":\"\",\"content\":\"   \",\"tags\":[\"skip\"]},{\"action\":\"add\",\"target_id\":\"\",\"content\":\"雨の日に頭痛が出やすい\",\"tags\":[\"health\",\"weather\"]}]}"}`))
	}))
	defer srv.Close()

	client, err := NewOpenAIClient(OpenAIClientConfig{
		APIKey:        "test-key",
		Model:         "test-model",
		Endpoint:      srv.URL,
		HTTPClient:    srv.Client(),
		MaxOperations: 1,
		MaxTags:       2,
	})
	if err != nil {
		t.Fatalf("NewOpenAIClient() error = %v", err)
	}
	got, err := client.CreateOperations(context.Background(), []types.ConversationRecord{{Role: types.RoleUser, Text: "test"}}, nil)
	if err != nil {
		t.Fatalf("CreateOperations() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("operations len = %d, want 1", len(got))
	}
	if got[0].Content != "朝はコーヒー" {
		t.Fatalf("Content = %q, want trimmed", got[0].Content)
	}
	wantTags := []string{"coffee", "morning"}
	if !sameStringSlice(got[0].Tags, wantTags) {
		t.Fatalf("Tags = %#v, want %#v", got[0].Tags, wantTags)
	}
}

func TestOpenAIClientCreateTagsSendsStructuredOutputSchema(t *testing.T) {
	var payload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output_text":"{\"tags\":[\" coffee \",\"Coffee\",\"morning\",\"routine\"]}"}`))
	}))
	defer srv.Close()

	client, err := NewOpenAIClient(OpenAIClientConfig{
		APIKey:     "test-key",
		Model:      "test-model",
		Endpoint:   srv.URL,
		HTTPClient: srv.Client(),
		MaxTags:    2,
	})
	if err != nil {
		t.Fatalf("NewOpenAIClient() error = %v", err)
	}
	got, err := client.CreateTags(context.Background(), "  ユーザーは朝にコーヒーを飲む  ")
	if err != nil {
		t.Fatalf("CreateTags() error = %v", err)
	}

	input := payload["input"].([]any)
	if len(input) != 2 {
		t.Fatalf("input len = %d, want 2", len(input))
	}
	if input[1].(map[string]any)["content"] != "ユーザーは朝にコーヒーを飲む" {
		t.Fatalf("user content = %#v", input[1])
	}
	format := payload["text"].(map[string]any)["format"].(map[string]any)
	if format["type"] != "json_schema" || format["name"] != "memory_tags" || format["strict"] != true {
		t.Fatalf("format = %#v", format)
	}
	schema, _ := json.Marshal(format["schema"])
	schemaText := string(schema)
	for _, want := range []string{`"tags"`, `"maxItems":2`, `"additionalProperties":false`} {
		if !strings.Contains(schemaText, want) {
			t.Fatalf("schema = %s, want it to contain %s", schemaText, want)
		}
	}
	if want := []string{"coffee", "morning"}; !sameStringSlice(got, want) {
		t.Fatalf("tags = %#v, want %#v", got, want)
	}
}

func TestOpenAIClientCreateTagsRejectsEmptyContent(t *testing.T) {
	client := &OpenAIClient{
		apiKey:   "test-key",
		model:    "test-model",
		endpoint: "http://127.0.0.1:1",
		client:   &http.Client{},
		maxTags:  DefaultMaxMemoryTags,
	}
	_, err := client.CreateTags(context.Background(), "   ")
	if err == nil || !strings.Contains(err.Error(), "content is required") {
		t.Fatalf("err = %v, want content is required", err)
	}
}

func TestOpenAIClientCreateTagsReturnsParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"output_text":"not json"}`))
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
	_, err = client.CreateTags(context.Background(), "ユーザーは朝にコーヒーを飲む")
	if err == nil || !strings.Contains(err.Error(), "parse tags") {
		t.Fatalf("err = %v, want it to contain parse tags", err)
	}
}

func TestOpenAIClientCreateOperationsReturnsErrors(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		body     string
		wantText string
	}{
		{name: "http error", status: http.StatusBadRequest, body: "bad request", wantText: "400 Bad Request"},
		{name: "response error", status: http.StatusOK, body: `{"error":{"message":"failed"}}`, wantText: "response failed: failed"},
		{name: "empty text", status: http.StatusOK, body: `{"output":[]}`, wantText: "response text is empty"},
		{name: "parse error", status: http.StatusOK, body: `{"output_text":"not json"}`, wantText: "parse operations"},
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
			_, err = client.CreateOperations(context.Background(), []types.ConversationRecord{{Role: types.RoleUser, Text: "test"}}, nil)
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

func TestNormalizeOperationsHandlesActions(t *testing.T) {
	got := normalizeOperations([]Operation{
		{Action: "ADD", TargetID: "ignored", Content: " 追加 ", Tags: []string{"a"}},
		{Action: "update", TargetID: "", Content: "IDなし更新"},
		{Action: "update", TargetID: " m-1 ", Content: "更新", Tags: []string{"b"}},
		{Action: "delete", TargetID: ""},
		{Action: "delete", TargetID: "m-2", Content: "無視される", Tags: []string{"c"}},
		{Action: "unknown", Content: "不明"},
		{Action: "", Content: "action省略は捨てる"},
	}, 10, 5)
	want := []Operation{
		{Action: OperationAdd, Content: "追加", Tags: []string{"a"}},
		{Action: OperationUpdate, TargetID: "m-1", Content: "更新", Tags: []string{"b"}},
		{Action: OperationDelete, TargetID: "m-2"},
	}
	if len(got) != len(want) {
		t.Fatalf("operations = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i].Action != want[i].Action || got[i].TargetID != want[i].TargetID || got[i].Content != want[i].Content || !sameStringSlice(got[i].Tags, want[i].Tags) {
			t.Fatalf("operations[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}
