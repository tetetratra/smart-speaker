package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	types "smart-speaker/internal/types"
)

type Config struct {
	APIKey       string
	Model        string
	Instructions string
	History      historyReader
	ToolSchemas  []any
	Client       responseClient
}

type responseClient interface {
	CreateResponseStream(ctx context.Context, messages []types.ChatMessage, systemContent string, onLine func(string) error) error
}

type historyReader interface {
	Snapshot() []types.ConversationRecord
}

type Client struct {
	apiKey   string
	model    string
	client   *http.Client
	endpoint string
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
	}, nil
}

func (c *Client) CreateResponseStream(ctx context.Context, messages []types.ChatMessage, systemContent string, onLine func(string) error) error {
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
		input = append(input, map[string]any{"role": role, "content": content})
	}
	if len(input) == 0 {
		return fmt.Errorf("llm: input is empty")
	}
	payload := map[string]any{
		"model":  c.model,
		"input":  input,
		"stream": true,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("llm: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	return readResponseStream(resp.Body, onLine)
}

func readResponseStream(r io.Reader, onLine func(string) error) error {
	reader := bufio.NewReader(r)
	var textBuffer strings.Builder
	for {
		raw, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return err
		}
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data != "" && data != "[DONE]" {
				var evt map[string]any
				if err := json.Unmarshal([]byte(data), &evt); err != nil {
					return err
				}
				if err := handleStreamEvent(evt, &textBuffer, onLine); err != nil {
					return err
				}
			}
		}
		if err == io.EOF {
			break
		}
	}
	if tail := strings.TrimSpace(textBuffer.String()); tail != "" && onLine != nil {
		return onLine(tail)
	}
	return nil
}

func handleStreamEvent(evt map[string]any, textBuffer *strings.Builder, onLine func(string) error) error {
	switch asString(evt["type"]) {
	case "response.output_text.delta":
		delta, _ := evt["delta"].(string)
		return appendStreamDelta(textBuffer, delta, onLine)
	case "response.failed":
		if errObj, ok := evt["error"].(map[string]any); ok {
			if msg := strings.TrimSpace(asString(errObj["message"])); msg != "" {
				return fmt.Errorf("llm: stream failed: %s", msg)
			}
		}
		return fmt.Errorf("llm: stream failed")
	}
	return nil
}

func appendStreamDelta(buffer *strings.Builder, delta string, onLine func(string) error) error {
	if delta == "" {
		return nil
	}
	buffer.WriteString(delta)
	for {
		current := buffer.String()
		idx := strings.IndexByte(current, '\n')
		if idx < 0 {
			return nil
		}
		line := strings.TrimSpace(current[:idx])
		rest := current[idx+1:]
		buffer.Reset()
		buffer.WriteString(rest)
		if line == "" {
			continue
		}
		if onLine != nil {
			if err := onLine(line); err != nil {
				return err
			}
		}
	}
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
