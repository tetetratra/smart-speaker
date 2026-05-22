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

func TestCreateResponseStreamSendsStructuredOutputSchema(t *testing.T) {
	var payload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"{\\\"items\\\":[\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"{\\\"type\\\":\\\"speech\\\",\\\"text\\\":\\\"はい\\\"}]}\"}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
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
	got, err := client.CreateResponseStream(context.Background(), []types.ChatMessage{{Role: types.RoleUser, Content: "こんにちは"}}, "system")
	if err != nil {
		t.Fatal(err)
	}
	if got != `{"items":[{"type":"speech","text":"はい"}]}` {
		t.Fatalf("response text = %q", got)
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

func TestReadResponseStreamReturnsWholeText(t *testing.T) {
	raw := strings.Join([]string{
		`data: {"type":"response.output_text.delta","delta":"{\"items\":["}`,
		`data: {"type":"response.output_text.delta","delta":"{\"type\":\"speech\",\"text\":\"1行目\n2行目\"}"}`,
		`data: {"type":"response.output_text.delta","delta":"]}"}`,
		`data: [DONE]`,
		``,
	}, "\n\n")
	got, err := readResponseStream(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if got != "{\"items\":[{\"type\":\"speech\",\"text\":\"1行目\n2行目\"}]}" {
		t.Fatalf("text = %q", got)
	}
}
