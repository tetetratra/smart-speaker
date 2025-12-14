package toolcaller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// SubAITool sends a query to a regular ChatCompletion and returns the text answer.
// 実際のWeb検索は行わず、通常モデルでの深い思考/回答を返す。
type SubAITool struct {
	apiKey string
	client *http.Client
	ctx    context.Context
}

func NewSubAITool(apiKey string) *SubAITool {
	if apiKey == "" {
		return nil
	}
	return &SubAITool{
		apiKey: apiKey,
		// Timeout 0 = no timeout (依頼に合わせて無制限相当)
		client: &http.Client{Timeout: 0},
	}
}

func (t *SubAITool) Name() string { return "sub_ai" }

func (t *SubAITool) SetContext(ctx context.Context) {
	t.ctx = ctx
}

func (t *SubAITool) Run(args map[string]any) (map[string]any, error) {
	if t == nil {
		return nil, fmt.Errorf("sub_ai tool not configured")
	}
	raw, ok := args["query"]
	if !ok {
		return nil, fmt.Errorf("missing query")
	}
	query, ok := raw.(string)
	if !ok || query == "" {
		return nil, fmt.Errorf("query must be a non-empty string")
	}

	reqBody := map[string]any{
		"model": "gpt-4o-mini",
		"messages": []map[string]any{
			{
				"role":    "system",
				"content": "あなたは調査・要約エージェントです。外部検索は行わず、既存知識と論理で簡潔に答えてください。",
			},
			{
				"role":    "user",
				"content": query,
			},
		},
		"temperature":       0.2,
		"max_output_tokens": 400,
		"top_p":             1,
		"stream":            false,
	}

	body, _ := json.Marshal(reqBody)
	ctx := t.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+t.apiKey)

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openai chat completion failed: %s", resp.Status)
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content any `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("openai chat completion returned no choices")
	}

	extract := func(v any) string {
		switch val := v.(type) {
		case string:
			return val
		case []any:
			var buf bytes.Buffer
			for _, part := range val {
				if m, ok := part.(map[string]any); ok {
					if s, ok := m["text"].(string); ok {
						buf.WriteString(s)
					}
				}
			}
			return buf.String()
		default:
			return fmt.Sprint(v)
		}
	}

	answer := extract(parsed.Choices[0].Message.Content)
	return map[string]any{"answer": answer}, nil
}
