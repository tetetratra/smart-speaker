package toolcaller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// SubAITool は Responses API + Web Search を使って調査・要約するツール。
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

	content := `あなたは調査・要約エージェントです。
- 回答に時間をかけすぎないでください
- 必要ならインターネット検索を行い、最新で正確な情報を提供してください
- 多くても3文以内にまとめてください
- [重要] 音声で再生されることを意識して、構造化せず、改行も入れず、URL等も出力せずに説明してください`

	reqBody := map[string]any{
		"model": "gpt-4.1",
		"input": []map[string]any{
			{"role": "system", "content": content},
			{"role": "user", "content": query},
		},
		"tools":             []map[string]any{{"type": "web_search"}},
		"temperature":       0.2,
		"max_output_tokens": 400,
		"stream":            false,
	}

	body, _ := json.Marshal(reqBody)
	ctx := t.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/responses", bytes.NewReader(body))
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
		msg, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai responses failed: %s: %s", resp.Status, string(msg))
	}

	var parsed map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}

	answer := extractResponseText(parsed)
	if answer == "" {
		return nil, fmt.Errorf("openai responses returned empty output")
	}
	return map[string]any{"answer": answer}, nil
}

// extractResponseText は Responses API の出力からテキストを抽出する。
func extractResponseText(parsed map[string]any) string {
	out, ok := parsed["output"].([]any)
	if !ok {
		return ""
	}
	var buf strings.Builder
	for _, item := range out {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		switch m["type"] {
		case "output_text":
			if s, ok := m["text"].(string); ok {
				buf.WriteString(s)
			}
		case "message":
			contents, _ := m["content"].([]any)
			for _, c := range contents {
				if cm, ok := c.(map[string]any); ok {
					if s, ok := cm["text"].(string); ok {
						buf.WriteString(s)
					} else if s, ok := cm["output_text"].(string); ok {
						buf.WriteString(s)
					}
				}
			}
		}
	}
	return strings.TrimSpace(buf.String())
}
