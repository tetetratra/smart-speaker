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
	"time"

	types "github.com/tetetratra/smart-speaker/internal/types"
)

const (
	DefaultOpenAIEndpoint         = "https://api.openai.com/v1/responses"
	DefaultMaxMemoryCandidates    = 5
	DefaultMaxMemoryCandidateTags = 5
)

type OpenAIClientConfig struct {
	APIKey        string
	Model         string
	Endpoint      string
	HTTPClient    *http.Client
	MaxCandidates int
	MaxTags       int
}

type OpenAIClient struct {
	apiKey        string
	model         string
	endpoint      string
	client        *http.Client
	maxCandidates int
	maxTags       int
}

type Candidate struct {
	Content string   `json:"content"`
	Tags    []string `json:"tags"`
}

func NewOpenAIClient(cfg OpenAIClientConfig) (*OpenAIClient, error) {
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("memory openai: API key is required")
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		return nil, fmt.Errorf("memory openai: model is required")
	}
	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		endpoint = DefaultOpenAIEndpoint
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("memory openai: endpoint is invalid")
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	maxCandidates := cfg.MaxCandidates
	if maxCandidates <= 0 {
		maxCandidates = DefaultMaxMemoryCandidates
	}
	maxTags := cfg.MaxTags
	if maxTags <= 0 {
		maxTags = DefaultMaxMemoryCandidateTags
	}
	return &OpenAIClient{
		apiKey:        apiKey,
		model:         model,
		endpoint:      endpoint,
		client:        httpClient,
		maxCandidates: maxCandidates,
		maxTags:       maxTags,
	}, nil
}

func (c *OpenAIClient) CreateCandidates(ctx context.Context, records []types.ConversationRecord) ([]Candidate, error) {
	if len(records) == 0 {
		return nil, nil
	}
	historyJSON, err := encodeHistory(records)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"model": c.model,
		"input": []map[string]any{
			{"role": "system", "content": memoryCandidateInstructions},
			{"role": "user", "content": historyJSON},
		},
		"text": memoryCandidateTextFormat(c.maxCandidates, c.maxTags),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("memory openai: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	text, err := readOpenAIResponseBody(resp.Body)
	if err != nil {
		return nil, err
	}
	var decoded candidateResponse
	if err := json.Unmarshal([]byte(text), &decoded); err != nil {
		return nil, fmt.Errorf("memory openai: parse candidates: %w", err)
	}
	return normalizeCandidates(decoded.Candidates, c.maxCandidates, c.maxTags), nil
}

const memoryCandidateInstructions = `Reset前の会話履歴から、後続会話で再利用できる長期記憶候補だけを抽出してください。
候補がない場合は candidates を空配列にしてください。
content は1候補につき1つの事実を、短く、主語が分かる自然文で書いてください。
tags は検索補助用の短いラベルで、固有名詞、カテゴリ、場所、習慣、健康、デバイス種別などを含めてください。
会話ログの生コピー、一時的な依頼、古くなりやすい状態、秘密情報らしき内容は保存候補にしないでください。

出力例:
- content: "ユーザーは平日の朝にコーヒーを飲むことが多い"
  tags: ["routine", "morning", "coffee", "weekday"]
- content: "ユーザーはリビングの照明操作に SwitchBot ハブミニを使っている"
  tags: ["SwitchBot", "smart_home", "living_room", "lighting", "device"]
- content: "ユーザーは雨の日に頭痛が出やすい"
  tags: ["health", "weather", "rain", "headache"]
- 保存すべき長期記憶候補がない場合:
  candidates: []`

type historyPayload struct {
	ID           string         `json:"id,omitempty"`
	Role         string         `json:"role"`
	Text         string         `json:"text,omitempty"`
	GenerationID int64          `json:"generation_id,omitempty"`
	Source       string         `json:"source,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	CreatedAt    string         `json:"created_at,omitempty"`
}

func encodeHistory(records []types.ConversationRecord) (string, error) {
	payload := make([]historyPayload, 0, len(records))
	for _, record := range records {
		role := strings.TrimSpace(record.Role)
		if role == "" {
			role = types.RoleUser
		}
		item := historyPayload{
			ID:           strings.TrimSpace(record.ID),
			Role:         role,
			Text:         strings.TrimSpace(record.Text),
			GenerationID: int64(record.GenerationID),
			Source:       strings.TrimSpace(record.Source),
			Metadata:     record.Metadata,
		}
		if !record.CreatedAt.IsZero() {
			item.CreatedAt = record.CreatedAt.UTC().Format(time.RFC3339Nano)
		}
		payload = append(payload, item)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("memory openai: encode history: %w", err)
	}
	return string(raw), nil
}

func memoryCandidateTextFormat(maxCandidates, maxTags int) map[string]any {
	return map[string]any{
		"format": map[string]any{
			"type":   "json_schema",
			"name":   "memory_candidates",
			"strict": true,
			"schema": memoryCandidateSchema(maxCandidates, maxTags),
		},
	}
}

func memoryCandidateSchema(maxCandidates, maxTags int) map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"candidates": map[string]any{
				"type":     "array",
				"maxItems": maxCandidates,
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"content": map[string]any{"type": "string"},
						"tags": map[string]any{
							"type":     "array",
							"maxItems": maxTags,
							"items":    map[string]any{"type": "string"},
						},
					},
					"required":             []string{"content", "tags"},
					"additionalProperties": false,
				},
			},
		},
		"required":             []string{"candidates"},
		"additionalProperties": false,
	}
}

type candidateResponse struct {
	Candidates []Candidate `json:"candidates"`
}

type openAIResponseBody struct {
	OutputText string             `json:"output_text"`
	Output     []openAIOutputItem `json:"output"`
	Error      *openAIError       `json:"error"`
}

type openAIError struct {
	Message string `json:"message"`
}

type openAIOutputItem struct {
	Content []openAIOutputContent `json:"content"`
}

type openAIOutputContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func readOpenAIResponseBody(r io.Reader) (string, error) {
	var body openAIResponseBody
	if err := json.NewDecoder(r).Decode(&body); err != nil {
		return "", err
	}
	if body.Error != nil {
		msg := strings.TrimSpace(body.Error.Message)
		if msg == "" {
			msg = "unknown error"
		}
		return "", fmt.Errorf("memory openai: response failed: %s", msg)
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
	return "", fmt.Errorf("memory openai: response text is empty")
}

func normalizeCandidates(candidates []Candidate, maxCandidates, maxTags int) []Candidate {
	if maxCandidates <= 0 {
		maxCandidates = DefaultMaxMemoryCandidates
	}
	if maxTags <= 0 {
		maxTags = DefaultMaxMemoryCandidateTags
	}
	out := make([]Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		content := strings.TrimSpace(candidate.Content)
		if content == "" {
			continue
		}
		out = append(out, Candidate{
			Content: content,
			Tags:    normalizeCandidateTags(candidate.Tags, maxTags),
		})
		if len(out) >= maxCandidates {
			break
		}
	}
	return out
}

func normalizeCandidateTags(tags []string, maxTags int) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		value := strings.TrimSpace(tag)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
		if len(out) >= maxTags {
			break
		}
	}
	return out
}
