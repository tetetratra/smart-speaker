package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/tetetratra/smart-speaker/internal/states/agentstatus"
	timerstate "github.com/tetetratra/smart-speaker/internal/states/timer"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

type Config struct {
	APIKey       string
	Model        string
	Instructions string
	History      historyReader
	AgentStatus  agentStatusReader
	Timers       timerSnapshotReader
	Memory       memoryContextProvider
	ToolSchemas  []any
	Client       responseClient
}

type responseClient interface {
	CreateResponse(ctx context.Context, messages []types.ChatMessage, systemContent string) (string, error)
}

type historyReader interface {
	Snapshot() []types.ConversationRecord
}

type agentStatusReader interface {
	Status() agentstatus.Status
}

type timerSnapshotReader interface {
	Snapshot() []timerstate.Timer
}

type memoryContextProvider interface {
	BuildContext(context.Context, []types.ConversationRecord) ([]types.ChatMessage, error)
}

type Client struct {
	apiKey   string
	model    string
	client   *http.Client
	endpoint string
	text     map[string]any
}

func NewClient(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("llm: API key is required")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, fmt.Errorf("llm: model is required")
	}
	return &Client{
		apiKey:   cfg.APIKey,
		model:    cfg.Model,
		client:   &http.Client{},
		endpoint: "https://api.openai.com/v1/responses",
		text:     timelineTextFormat(cfg.ToolSchemas),
	}, nil
}

func (c *Client) CreateResponse(ctx context.Context, messages []types.ChatMessage, systemContent string) (string, error) {
	input := []map[string]any{}
	if strings.TrimSpace(systemContent) != "" {
		input = append(input, map[string]any{"role": "system", "content": systemContent})
	}
	for _, msg := range messages {
		role := strings.TrimSpace(msg.Role)
		if role == "" {
			role = types.RoleUser
		}
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		input = append(input, map[string]any{"role": transportRole(role), "content": encodeHistoryContent(role, content)})
	}
	if len(input) == 0 {
		return "", fmt.Errorf("llm: input is empty")
	}
	payload := map[string]any{
		"model": c.model,
		"input": input,
	}
	if c.text != nil {
		payload["text"] = c.text
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
		return "", fmt.Errorf("llm: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	return readResponseBody(resp.Body)
}

func transportRole(role string) string {
	if strings.TrimSpace(role) == "system" {
		return "system"
	}
	return types.RoleUser
}

func encodeHistoryContent(role, content string) string {
	payload := map[string]any{
		"role":    strings.TrimSpace(role),
		"content": json.RawMessage(content),
	}
	if payload["role"] == "" {
		payload["role"] = types.RoleUser
	}
	encoded, err := json.Marshal(payload)
	if err == nil {
		return string(encoded)
	}
	payload["content"] = content
	encoded, err = json.Marshal(payload)
	if err != nil {
		return content
	}
	return string(encoded)
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

func readResponseBody(r io.Reader) (string, error) {
	var body responseBody
	if err := json.NewDecoder(r).Decode(&body); err != nil {
		return "", err
	}
	return extractResponseText(body)
}

func extractResponseText(body responseBody) (string, error) {
	if body.Error != nil {
		msg := strings.TrimSpace(body.Error.Message)
		if msg == "" {
			msg = "unknown error"
		}
		return "", fmt.Errorf("llm: response failed: %s", msg)
	}
	if text := strings.TrimSpace(body.OutputText); text != "" {
		return text, nil
	}
	var parts []string
	for _, item := range body.Output {
		for _, content := range item.Content {
			if content.Type != "output_text" {
				continue
			}
			if text := strings.TrimSpace(content.Text); text != "" {
				parts = append(parts, text)
			}
		}
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("llm: response text is empty")
	}
	return parts[0], nil
}

func appendCurrentTimestamp(prompt string) string {
	now := time.Now()
	dateLine := "現在日時：" + now.Format("2006/01/02") + " (" + formatJapaneseWeekday(now.Weekday()) + ")"
	timeLine := "現在時刻：" + now.Format("15:04:05")
	if strings.TrimSpace(prompt) == "" {
		return dateLine + "\n" + timeLine
	}
	return strings.TrimRight(prompt, "\n") + "\n" + dateLine + "\n" + timeLine
}

func appendTimerSnapshot(prompt string, timers []timerstate.Timer) string {
	var lines []string
	lines = append(lines, "現在の未到達タイマー一覧:")
	if len(timers) == 0 {
		lines = append(lines, "- なし")
	} else {
		for _, timer := range timers {
			lines = append(lines, fmt.Sprintf("- id=%s at=%s action=%s", timer.ID, timer.At.Format(time.RFC3339), timer.Action))
		}
	}
	block := strings.Join(lines, "\n")
	if strings.TrimSpace(prompt) == "" {
		return block
	}
	return strings.TrimRight(prompt, "\n") + "\n" + block
}

func formatJapaneseWeekday(w time.Weekday) string {
	switch w {
	case time.Monday:
		return "月"
	case time.Tuesday:
		return "火"
	case time.Wednesday:
		return "水"
	case time.Thursday:
		return "木"
	case time.Friday:
		return "金"
	case time.Saturday:
		return "土"
	case time.Sunday:
		return "日"
	default:
		return ""
	}
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
