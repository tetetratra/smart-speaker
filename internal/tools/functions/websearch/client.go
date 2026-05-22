package websearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type ClientConfig struct {
	APIKey     string
	Model      string
	HTTPClient *http.Client
	Endpoint   string
}

type Client struct {
	apiKey   string
	model    string
	client   *http.Client
	endpoint string
}

func NewClient(cfg ClientConfig) *Client {
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1/responses"
	}
	return &Client{
		apiKey:   strings.TrimSpace(cfg.APIKey),
		model:    strings.TrimSpace(cfg.Model),
		client:   httpClient,
		endpoint: endpoint,
	}
}

func (c *Client) Search(ctx context.Context, query string) (string, error) {
	if strings.TrimSpace(c.apiKey) == "" {
		return "", fmt.Errorf("web_search: API key is required")
	}
	if strings.TrimSpace(c.model) == "" {
		return "", fmt.Errorf("web_search: model is required")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return "", fmt.Errorf("web_search: query is required")
	}

	payload := map[string]any{
		"model": c.model,
		"input": query,
		"tools": []map[string]any{
			{"type": "web_search"},
		},
		"include": []string{"web_search_call.action.sources"},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("web_search: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	result, err := readSearchResponse(resp.Body)
	if err != nil {
		return "", err
	}
	return result, nil
}

type responseBody struct {
	OutputText string               `json:"output_text"`
	Output     []responseOutputItem `json:"output"`
	Error      *responseError       `json:"error"`
}

type responseError struct {
	Message string `json:"message"`
}

type responseOutputItem struct {
	Type    string                  `json:"type"`
	Content []responseOutputContent `json:"content"`
}

type responseOutputContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func readSearchResponse(r io.Reader) (string, error) {
	var body responseBody
	if err := json.NewDecoder(r).Decode(&body); err != nil {
		return "", err
	}
	if body.Error != nil {
		msg := strings.TrimSpace(body.Error.Message)
		if msg == "" {
			msg = "unknown error"
		}
		return "", fmt.Errorf("web_search: response failed: %s", msg)
	}
	if text := strings.TrimSpace(body.OutputText); text != "" {
		return text, nil
	}
	for _, item := range body.Output {
		for _, content := range item.Content {
			if content.Type != "output_text" {
				continue
			}
			if text := strings.TrimSpace(content.Text); text != "" {
				return text, nil
			}
		}
	}
	return "", fmt.Errorf("web_search: result is empty")
}
