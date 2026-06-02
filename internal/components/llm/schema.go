package llm

const setWhiteboardToolName = "set_whiteboard"

func timelineTextFormat(toolSchemas []any) map[string]any {
	return map[string]any{
		"format": map[string]any{
			"type":   "json_schema",
			"name":   "conversation_timeline",
			"strict": true,
			"schema": timelineSchema(toolSchemas),
		},
	}
}

func setWhiteboardFieldSchema() map[string]any {
	schema := strictObject(map[string]any{
		"content": map[string]any{
			"type": "string",
		},
	})
	// Strict JSON schema requires every property key in required.
	// Use null when the LLM does not update the whiteboard.
	schema["type"] = []string{"object", "null"}
	return schema
}

func timelineSchema(toolSchemas []any) map[string]any {
	itemSchemas := []any{speechItemSchema(), waitItemSchema()}
	for _, toolSchema := range toolSchemas {
		if def, ok := toolSchema.(map[string]any); ok {
			if name, ok := def["name"].(string); ok && name == setWhiteboardToolName {
				continue
			}
		}
		if schema := toolItemSchema(toolSchema); schema != nil {
			itemSchemas = append(itemSchemas, schema)
		}
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"items": map[string]any{
				"type": "array",
				"items": map[string]any{
					"anyOf": itemSchemas,
				},
				"maxItems": maxTimelineItems,
			},
			"set_whiteboard": setWhiteboardFieldSchema(),
		},
		"required":             []string{"items", "set_whiteboard"},
		"additionalProperties": false,
	}
}

func speechItemSchema() map[string]any {
	return strictObject(map[string]any{
		"type": map[string]any{
			"type": "string",
			"enum": []string{"speech"},
		},
		"text": map[string]any{
			"type": "string",
		},
	})
}

func waitItemSchema() map[string]any {
	return strictObject(map[string]any{
		"type": map[string]any{
			"type": "string",
			"enum": []string{"wait"},
		},
		"sec": map[string]any{
			"type": "number",
		},
	})
}

func toolItemSchema(raw any) map[string]any {
	def, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	name, ok := def["name"].(string)
	if !ok || name == "" {
		return nil
	}
	argsSchema := strictToolArgsSchema(def["parameters"])
	return strictObject(map[string]any{
		"type": map[string]any{
			"type": "string",
			"enum": []string{"tool"},
		},
		"name": map[string]any{
			"type": "string",
			"enum": []string{name},
		},
		"args": argsSchema,
	})
}

func strictToolArgsSchema(raw any) map[string]any {
	params, ok := raw.(map[string]any)
	if !ok {
		return strictObject(map[string]any{})
	}
	props, _ := params["properties"].(map[string]any)
	strictProps := make(map[string]any, len(props))
	for name, prop := range props {
		strictProps[name] = nullableIfOptional(cleanPropertySchema(prop), name, stringSet(params["required"]))
	}
	return map[string]any{
		"type":                 "object",
		"properties":           strictProps,
		"required":             sortedKeys(strictProps),
		"additionalProperties": false,
	}
}

func cleanPropertySchema(raw any) map[string]any {
	prop, ok := raw.(map[string]any)
	if !ok {
		return map[string]any{"type": "string"}
	}
	cleaned := map[string]any{}
	if typ, ok := prop["type"]; ok {
		cleaned["type"] = typ
	} else {
		cleaned["type"] = "string"
	}
	if enumValues, ok := prop["enum"]; ok {
		cleaned["enum"] = enumValues
	}
	if description, ok := prop["description"]; ok {
		cleaned["description"] = description
	}
	return cleaned
}

func nullableIfOptional(schema map[string]any, name string, required map[string]struct{}) map[string]any {
	if _, ok := required[name]; ok {
		return schema
	}
	out := cloneMap(schema)
	switch typ := out["type"].(type) {
	case string:
		out["type"] = []string{typ, "null"}
	case []string:
		for _, t := range typ {
			if t == "null" {
				return out
			}
		}
		out["type"] = append(append([]string{}, typ...), "null")
	case []any:
		for _, t := range typ {
			if s, ok := t.(string); ok && s == "null" {
				return out
			}
		}
		out["type"] = append(append([]any{}, typ...), "null")
	}
	return out
}

func strictObject(properties map[string]any) map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             sortedKeys(properties),
		"additionalProperties": false,
	}
}

func stringSet(raw any) map[string]struct{} {
	out := map[string]struct{}{}
	switch values := raw.(type) {
	case []string:
		for _, value := range values {
			out[value] = struct{}{}
		}
	case []any:
		for _, value := range values {
			if s, ok := value.(string); ok {
				out[s] = struct{}{}
			}
		}
	}
	return out
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sortStrings(keys)
	return keys
}

func cloneMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
