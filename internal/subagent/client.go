package subagent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Answer struct{ ID, Text string }
type Client interface {
	Respond(context.Context, string, string) (Answer, error)
}
type OpenAIClient struct {
	APIKey, Model, Endpoint string
	HTTP                    *http.Client
}

func (c *OpenAIClient) Respond(ctx context.Context, input, previous string) (Answer, error) {
	endpoint := c.Endpoint
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1/responses"
	}
	payload := map[string]any{"model": c.Model, "input": input, "store": true, "reasoning": map[string]any{"effort": "high"}, "max_output_tokens": 8192, "instructions": "依頼された複雑な質問・作業を慎重に検討し、日本語のテキストで回答してください。不明点や推測は明示してください。外部ツールや実環境を操作したと装わないでください。"}
	if previous != "" {
		payload["previous_response_id"] = previous
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Answer{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Answer{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Minute}
	}
	resp, err := client.Do(req)
	if err != nil {
		return Answer{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Answer{}, fmt.Errorf("サブエージェントAPIが失敗しました (HTTP %d)", resp.StatusCode)
	}
	var result struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Output []struct {
			Type    string                        `json:"type"`
			Content []struct{ Type, Text string } `json:"content"`
		} `json:"output"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&result); err != nil {
		return Answer{}, err
	}
	if result.Status != "completed" || result.ID == "" {
		return Answer{}, fmt.Errorf("サブエージェントの応答が完了しませんでした")
	}
	var texts []string
	for _, item := range result.Output {
		if item.Type != "message" {
			continue
		}
		for _, content := range item.Content {
			if content.Type == "output_text" {
				texts = append(texts, content.Text)
			}
		}
	}
	text := strings.TrimSpace(strings.Join(texts, "\n"))
	if text == "" {
		return Answer{}, fmt.Errorf("サブエージェントの回答が空でした")
	}
	return Answer{ID: result.ID, Text: text}, nil
}
