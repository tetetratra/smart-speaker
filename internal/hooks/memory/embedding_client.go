package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const (
	DefaultEmbeddingBaseURL = "http://embedding:80"
	DefaultEmbeddingModel   = "intfloat/multilingual-e5-small"
)

type EmbeddingClientConfig struct {
	BaseURL    string
	PromptName string
	HTTPClient *http.Client
}

type EmbeddingClient struct {
	baseURL    string
	promptName string
	client     *http.Client
}

func NewEmbeddingClient(cfg EmbeddingClientConfig) (*EmbeddingClient, error) {
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		baseURL = DefaultEmbeddingBaseURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("memory embedding: base URL is invalid")
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{}
	}

	return &EmbeddingClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		promptName: strings.TrimSpace(cfg.PromptName),
		client:     httpClient,
	}, nil
}

func (c *EmbeddingClient) Embed(ctx context.Context, text string) ([]float64, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("memory embedding: text is required")
	}

	payload := embedRequest{Inputs: text}
	if c.promptName != "" {
		payload.PromptName = c.promptName
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/embed", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("memory embedding: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}

	var embeddings [][]float64
	if err := json.NewDecoder(resp.Body).Decode(&embeddings); err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("memory embedding: response is empty")
	}
	if len(embeddings[0]) == 0 {
		return nil, fmt.Errorf("memory embedding: embedding is empty")
	}

	out := make([]float64, len(embeddings[0]))
	copy(out, embeddings[0])
	return out, nil
}

type embedRequest struct {
	Inputs     string `json:"inputs"`
	PromptName string `json:"prompt_name,omitempty"`
}
