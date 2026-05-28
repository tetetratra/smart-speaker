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
	if strings.Contains(schemaText, `"name":"set_whiteboard"`) {
		t.Fatalf("schema should not include set_whiteboard as items tool, got: %s", schemaText)
	}
	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatalf("required = %T, want []string", schema["required"])
	}
	for _, want := range []string{"items", "set_whiteboard"} {
		found := false
		for _, name := range required {
			if name == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("required = %v, want it to include %s", required, want)
		}
	}
	setWB, ok := schema["properties"].(map[string]any)["set_whiteboard"].(map[string]any)
	if !ok {
		t.Fatal("set_whiteboard property schema missing")
	}
	if typ, ok := setWB["type"].([]string); !ok || len(typ) != 2 || typ[0] != "object" || typ[1] != "null" {
		t.Fatalf("set_whiteboard type = %#v, want [object null]", setWB["type"])
	}
}
