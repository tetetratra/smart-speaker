package responsesapi

import (
	"bufio"
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
	Tools        []any
}

type Client struct {
	apiKey   string
	model    string
	client   *http.Client
	tools    []any
	endpoint string
}

func NewClient(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("responsesapi: API key is required")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, fmt.Errorf("responsesapi: model is required")
	}
	return &Client{
		apiKey:   cfg.APIKey,
		model:    cfg.Model,
		client:   &http.Client{},
		tools:    cfg.Tools,
		endpoint: "https://api.openai.com/v1/responses",
	}, nil
}

func (c *Client) CreateResponse(ctx context.Context, messages []types.ChatMessage, systemContent string, toolChoice any, toolsOverride []any) (types.ResponsesResponse, error) {
	input := []map[string]any{}
	if strings.TrimSpace(systemContent) != "" {
		input = append(input, map[string]any{
			"role":    "system",
			"content": systemContent,
		})
	}
	for _, msg := range messages {
		role := strings.TrimSpace(msg.Role)
		if role == "" {
			role = "user"
		}
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		input = append(input, map[string]any{
			"role":    role,
			"content": content,
		})
	}
	if len(input) == 0 {
		return types.ResponsesResponse{}, fmt.Errorf("responsesapi: input is empty")
	}

	payload := map[string]any{
		"model": c.model,
		"input": input,
	}
	tools := c.tools
	if toolsOverride != nil {
		tools = toolsOverride
	}
	if len(tools) > 0 {
		payload["tools"] = tools
	}
	if toolChoice != nil {
		payload["tool_choice"] = toolChoice
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return types.ResponsesResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
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
	toolCalls := extractToolCalls(parsed)
	return types.ResponsesResponse{
		Text:        textOut,
		ResponseID:  respID,
		HasResponse: strings.TrimSpace(textOut) != "",
		ToolCalls:   toolCalls,
	}, nil
}

func (c *Client) CreateResponseStream(ctx context.Context, messages []types.ChatMessage, systemContent string, toolChoice any, toolsOverride []any, onLine func(string) error) (types.ResponsesResponse, error) {
	input := []map[string]any{}
	if strings.TrimSpace(systemContent) != "" {
		input = append(input, map[string]any{
			"role":    "system",
			"content": systemContent,
		})
	}
	for _, msg := range messages {
		role := strings.TrimSpace(msg.Role)
		if role == "" {
			role = "user"
		}
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		input = append(input, map[string]any{
			"role":    role,
			"content": content,
		})
	}
	if len(input) == 0 {
		return types.ResponsesResponse{}, fmt.Errorf("responsesapi: input is empty")
	}

	payload := map[string]any{
		"model":  c.model,
		"input":  input,
		"stream": true,
	}
	tools := c.tools
	if toolsOverride != nil {
		tools = toolsOverride
	}
	if len(tools) > 0 {
		payload["tools"] = tools
	}
	if toolChoice != nil {
		payload["tool_choice"] = toolChoice
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return types.ResponsesResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
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

	stream, err := readResponseStream(resp.Body, onLine)
	if err != nil {
		return types.ResponsesResponse{}, err
	}
	return stream, nil
}

func (c *Client) SubmitToolOutput(ctx context.Context, previousResponseID, callID, output string, toolsOverride []any) (types.ResponsesResponse, error) {
	if strings.TrimSpace(previousResponseID) == "" {
		return types.ResponsesResponse{}, fmt.Errorf("responsesapi: previous response id is required")
	}
	item := map[string]any{
		"type":    "function_call_output",
		"call_id": callID,
		"output":  output,
	}
	input := []map[string]any{item}
	payload := map[string]any{
		"model":                c.model,
		"input":                input,
		"previous_response_id": previousResponseID,
	}
	tools := c.tools
	if toolsOverride != nil {
		tools = toolsOverride
	}
	if len(tools) > 0 {
		payload["tools"] = tools
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return types.ResponsesResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
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
	toolCalls := extractToolCalls(parsed)
	return types.ResponsesResponse{
		Text:        textOut,
		ResponseID:  respID,
		HasResponse: strings.TrimSpace(textOut) != "",
		ToolCalls:   toolCalls,
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

func readResponseStream(r io.Reader, onLine func(string) error) (types.ResponsesResponse, error) {
	reader := bufio.NewReader(r)
	var textBuffer strings.Builder
	var completed map[string]any
	for {
		raw, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return types.ResponsesResponse{}, err
		}
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data != "" && data != "[DONE]" {
				var evt map[string]any
				if err := json.Unmarshal([]byte(data), &evt); err != nil {
					return types.ResponsesResponse{}, err
				}
				if err := handleStreamEvent(evt, &textBuffer, &completed, onLine); err != nil {
					return types.ResponsesResponse{}, err
				}
			}
		}
		if err == io.EOF {
			break
		}
	}
	if tail := strings.TrimSpace(textBuffer.String()); tail != "" {
		if onLine != nil {
			if err := onLine(tail); err != nil {
				return types.ResponsesResponse{}, err
			}
		}
	}
	if completed == nil {
		return types.ResponsesResponse{}, nil
	}
	textOut := extractResponseText(completed)
	respID, _ := completed["id"].(string)
	toolCalls := extractToolCalls(completed)
	return types.ResponsesResponse{
		Text:        textOut,
		ResponseID:  respID,
		HasResponse: strings.TrimSpace(textOut) != "",
		ToolCalls:   toolCalls,
	}, nil
}

func handleStreamEvent(evt map[string]any, textBuffer *strings.Builder, completed *map[string]any, onLine func(string) error) error {
	switch asString(evt["type"]) {
	case "response.output_text.delta":
		delta, _ := evt["delta"].(string)
		return appendStreamDelta(textBuffer, delta, onLine)
	case "response.completed":
		if response, ok := evt["response"].(map[string]any); ok {
			*completed = response
		}
	case "response.failed":
		if errObj, ok := evt["error"].(map[string]any); ok {
			if msg := strings.TrimSpace(asString(errObj["message"])); msg != "" {
				return fmt.Errorf("responsesapi: stream failed: %s", msg)
			}
		}
		return fmt.Errorf("responsesapi: stream failed")
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

func extractToolCalls(parsed map[string]any) []types.ToolRequest {
	respID, _ := parsed["id"].(string)
	output, ok := parsed["output"].([]any)
	if !ok {
		return nil
	}
	var calls []types.ToolRequest
	for _, item := range output {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		switch asString(entry["type"]) {
		case "function_call":
			if call := parseFunctionCall(entry, respID); call != nil {
				calls = append(calls, *call)
			}
		}
	}
	return calls
}

func parseFunctionCall(entry map[string]any, respID string) *types.ToolRequest {
	name, ok := entry["name"].(string)
	if !ok || name == "" {
		return nil
	}
	callID, _ := entry["call_id"].(string)
	args := ""
	switch v := entry["arguments"].(type) {
	case string:
		args = v
	case map[string]any:
		if encoded, err := json.Marshal(v); err == nil {
			args = string(encoded)
		}
	case nil:
		args = ""
	}
	if callID == "" {
		return nil
	}
	if strings.TrimSpace(args) == "" {
		args = "{}"
	}
	return &types.ToolRequest{
		ResponseID: respID,
		ToolCallID: callID,
		Name:       name,
		Arguments:  json.RawMessage(args),
	}
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
