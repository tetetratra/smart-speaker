package llm

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTimelineSchemaSetWhiteboardAtRootNotInItems(t *testing.T) {
	schema := timelineSchema([]any{
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
	encoded, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	schemaText := string(encoded)
	for _, want := range []string{`"set_whiteboard"`, `"content"`, `"items"`, `"web_search"`} {
		if !strings.Contains(schemaText, want) {
			t.Fatalf("schema = %s, want it to contain %s", schemaText, want)
		}
	}
	// set_whiteboard appears as root property, not as tool name enum inside items anyOf
	if strings.Count(schemaText, `"set_whiteboard"`) != 1 {
		t.Fatalf("schema should contain set_whiteboard once at root, got: %s", schemaText)
	}
}
