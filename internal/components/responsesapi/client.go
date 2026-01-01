package responsesapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	types "smart-speaker/internal/types"
)

// Config はResponses APIクライアントの設定を表します。
type Config struct {
	APIKey       string
	Model        string
	Instructions string
}

type Client struct {
	apiKey string
	model  string
	instr  string
	client *http.Client
}

func NewClient(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("responsesapi: API key is required")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, fmt.Errorf("responsesapi: model is required")
	}
	return &Client{
		apiKey: cfg.APIKey,
		model:  cfg.Model,
		instr:  cfg.Instructions,
		client: &http.Client{},
	}, nil
}

func (c *Client) CreateResponse(ctx context.Context, text string) (types.ResponsesResponse, error) {
	input := []map[string]any{}
	if strings.TrimSpace(c.instr) != "" {
		input = append(input, map[string]any{
			"role":    "system",
			"content": c.instr,
		})
	}
	input = append(input, map[string]any{
		"role":    "user",
		"content": text,
	})

	payload := map[string]any{
		"model": c.model,
		"input": input,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return types.ResponsesResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/responses", bytes.NewReader(body))
	if err != nil {
		return types.ResponsesResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return types.ResponsesResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(resp.Body)
		return types.ResponsesResponse{}, fmt.Errorf("responsesapi: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}

	var parsed map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return types.ResponsesResponse{}, err
	}
	textOut := extractResponseText(parsed)
	respID, _ := parsed["id"].(string)
	return types.ResponsesResponse{
		Text:        textOut,
		ResponseID:  respID,
		HasResponse: strings.TrimSpace(textOut) != "",
	}, nil
}

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
