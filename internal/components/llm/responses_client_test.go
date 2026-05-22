package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	types "smart-speaker/internal/types"
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

func TestReadResponseBodyFailsWhenMultipleOutputTextParts(t *testing.T) {
	raw := `{
		"output": [
			{"type": "message", "content": [{"type": "output_text", "text": "{\"items\":[]}" }]},
			{"type": "message", "content": [{"type": "output_text", "text": "{\"items\":[]}" }]}
		]
	}`
	_, err := readResponseBody(strings.NewReader(raw))
	if err == nil || !strings.Contains(err.Error(), "multiple output_text parts") {
		t.Fatalf("err = %v, want multiple output_text parts", err)
	}
}
