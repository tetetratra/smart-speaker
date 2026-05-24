package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	types "github.com/tetetratra/smart-speaker/internal/types"
)

func TestCreateResponseSendsStructuredOutputSchema(t *testing.T) {
	var payload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"output": [
				{
					"type": "message",
					"role": "assistant",
					"content": [
						{"type": "output_text", "text": "{\"items\":[{\"type\":\"speech\",\"text\":\"はい\"}]}"}
					]
				}
			]
		}`))
	}))
	defer srv.Close()

	client := &Client{
		apiKey:   "test-key",
		model:    "test-model",
		client:   srv.Client(),
		endpoint: srv.URL,
		text: timelineTextFormat([]any{
			map[string]any{
				"type": "function",
				"name": "set_whiteboard",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"content": map[string]any{"type": "string"},
					},
					"required":             []string{"content"},
					"additionalProperties": false,
				},
			},
		}),
	}
	got, err := client.CreateResponse(context.Background(), []types.ChatMessage{{Role: types.RoleUser, Content: "こんにちは"}}, "system")
	if err != nil {
		t.Fatal(err)
	}
	if got != `{"items":[{"type":"speech","text":"はい"}]}` {
		t.Fatalf("response text = %q", got)
	}
	if _, ok := payload["stream"]; ok {
		t.Fatalf("payload stream = %#v, want omitted", payload["stream"])
	}
	input := payload["input"].([]any)
	message := input[1].(map[string]any)
	if message["role"] != types.RoleUser {
		t.Fatalf("transport role = %v, want user", message["role"])
	}
	var content map[string]any
	if err := json.Unmarshal([]byte(message["content"].(string)), &content); err != nil {
		t.Fatalf("history content JSON: %v", err)
	}
	if content["role"] != types.RoleUser {
		t.Fatalf("history role = %v, want user", content["role"])
	}
	text, ok := payload["text"].(map[string]any)
	if !ok {
		t.Fatalf("payload text = %#v", payload["text"])
	}
	format, ok := text["format"].(map[string]any)
	if !ok {
		t.Fatalf("text format = %#v", text["format"])
	}
	if format["type"] != "json_schema" || format["name"] != "conversation_timeline" || format["strict"] != true {
		t.Fatalf("format = %#v", format)
	}
	encoded, _ := json.Marshal(format["schema"])
	schemaText := string(encoded)
	for _, want := range []string{`"items"`, `"speech"`, `"wait"`, `"tool"`, `"set_whiteboard"`} {
		if !strings.Contains(schemaText, want) {
			t.Fatalf("schema = %s, want it to contain %s", schemaText, want)
		}
	}
}

func TestCreateResponseWrapsAppRolesForResponsesAPI(t *testing.T) {
	var payload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output_text":"{\"items\":[]}"}`))
	}))
	defer srv.Close()

	client := &Client{
		apiKey:   "test-key",
		model:    "test-model",
		client:   srv.Client(),
		endpoint: srv.URL,
	}
	_, err := client.CreateResponse(context.Background(), []types.ChatMessage{
		{Role: types.RoleAgent, Content: `{"type":"message","text":"確認します"}`},
		{Role: types.RoleToolCall, Content: `{"type":"tool_call","tool_call_id":"call-1","arguments":{"place":"living"}}`},
		{Role: types.RoleToolResult, Content: `{"type":"tool_result","tool_call_id":"call-1","output":{"temp":29}}`},
	}, "system")
	if err != nil {
		t.Fatal(err)
	}
	input := payload["input"].([]any)
	if len(input) != 4 {
		t.Fatalf("input len = %d, want 4", len(input))
	}
	for i := 1; i < len(input); i++ {
		item := input[i].(map[string]any)
		if item["role"] != types.RoleUser {
			t.Fatalf("input[%d].role = %v, want user transport role", i, item["role"])
		}
	}
	var wrapped map[string]any
	if err := json.Unmarshal([]byte(input[2].(map[string]any)["content"].(string)), &wrapped); err != nil {
		t.Fatalf("wrapped content JSON: %v", err)
	}
	if wrapped["role"] != types.RoleToolCall {
		t.Fatalf("wrapped role = %v, want tool_call", wrapped["role"])
	}
	content := wrapped["content"].(map[string]any)
	if content["type"] != "tool_call" || content["tool_call_id"] != "call-1" {
		t.Fatalf("wrapped content = %#v", content)
	}
}

func TestTimelineSchemaIncludesWebSearchQueryOnly(t *testing.T) {
	text := timelineTextFormat([]any{
		map[string]any{
			"type": "function",
			"name": "web_search",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
				},
				"required":             []string{"query"},
				"additionalProperties": false,
			},
		},
	})
	encoded, _ := json.Marshal(text)
	schemaText := string(encoded)
	for _, want := range []string{`"web_search"`, `"query"`} {
		if !strings.Contains(schemaText, want) {
			t.Fatalf("schema = %s, want it to contain %s", schemaText, want)
		}
	}
	for _, unwanted := range []string{`"context"`, `"max_results"`} {
		if strings.Contains(schemaText, unwanted) {
			t.Fatalf("schema = %s, want it not to contain %s", schemaText, unwanted)
		}
	}
}

func TestReadResponseBodyReturnsOutputTextContent(t *testing.T) {
	raw := `{
		"output": [
			{
				"type": "message",
				"role": "assistant",
				"content": [
					{"type": "output_text", "text": "{\"items\":[{\"type\":\"speech\",\"text\":\"1行目\\n2行目\"}]}"}
				]
			}
		]
	}`
	got, err := readResponseBody(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if got != `{"items":[{"type":"speech","text":"1行目\n2行目"}]}` {
		t.Fatalf("text = %q", got)
	}
}

func TestReadResponseBodyPrefersOutputTextWhenPresent(t *testing.T) {
	raw := `{
		"output_text": "{\"items\":[{\"type\":\"speech\",\"text\":\"上書き\"}]}",
		"output": [
			{
				"type": "message",
				"content": [
					{"type": "output_text", "text": "{\"items\":[{\"type\":\"speech\",\"text\":\"未使用\"}]}"}
				]
			}
		]
	}`
	got, err := readResponseBody(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if got != `{"items":[{"type":"speech","text":"上書き"}]}` {
		t.Fatalf("text = %q", got)
	}
}

func TestReadResponseBodyFailsWhenTextIsEmpty(t *testing.T) {
	_, err := readResponseBody(strings.NewReader(`{"output":[]}`))
	if err == nil || !strings.Contains(err.Error(), "response text is empty") {
		t.Fatalf("err = %v, want response text is empty", err)
	}
}

func TestReadResponseBodyUsesFirstOutputTextPart(t *testing.T) {
	raw := `{
		"output": [
			{"type": "message", "content": [{"type": "output_text", "text": "{\"items\":[{\"type\":\"speech\",\"text\":\"1つ目\"}]}" }]},
			{"type": "message", "content": [{"type": "output_text", "text": "{\"items\":[{\"type\":\"speech\",\"text\":\"2つ目\"}]}" }]}
		]
	}`
	got, err := readResponseBody(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if got != `{"items":[{"type":"speech","text":"1つ目"}]}` {
		t.Fatalf("text = %q", got)
	}
}
