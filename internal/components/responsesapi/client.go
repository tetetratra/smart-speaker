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
	Tools        []any
}

type Client struct {
	apiKey string
	model  string
	instr  string
	client *http.Client
	tools  []any
}

func NewClient(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("responsesapi: API key is required")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, fmt.Errorf("responsesapi: model is required")
	}
	tools := cfg.Tools
	if tools == nil {
		tools = defaultTools()
	}
	return &Client{
		apiKey: cfg.APIKey,
		model:  cfg.Model,
		instr:  cfg.Instructions,
		client: &http.Client{},
		tools:  tools,
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
	if len(c.tools) > 0 {
		payload["tools"] = c.tools
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
	toolCalls := extractToolCalls(parsed)
	return types.ResponsesResponse{
		Text:        textOut,
		ResponseID:  respID,
		HasResponse: strings.TrimSpace(textOut) != "",
		ToolCalls:   toolCalls,
	}, nil
}

func (c *Client) SubmitToolOutput(ctx context.Context, previousResponseID, callID, output string) (types.ResponsesResponse, error) {
	if strings.TrimSpace(previousResponseID) == "" {
		return types.ResponsesResponse{}, fmt.Errorf("responsesapi: previous response id is required")
	}
	item := map[string]any{
		"type":    "function_call_output",
		"call_id": callID,
		"output":  output,
	}
	input := []map[string]any{item}
	if strings.TrimSpace(c.instr) != "" {
		input = append([]map[string]any{{
			"type":    "message",
			"role":    "system",
			"content": []map[string]any{{"type": "input_text", "text": c.instr}},
		}}, input...)
	}
	payload := map[string]any{
		"model":                c.model,
		"input":                input,
		"previous_response_id": previousResponseID,
	}
	if len(c.tools) > 0 {
		payload["tools"] = c.tools
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
		case "tool_calls":
			if arr, ok := entry["tool_calls"].([]any); ok {
				for _, call := range arr {
					if m, ok := call.(map[string]any); ok {
						if parsedCall := parseFunctionCall(m, respID); parsedCall != nil {
							calls = append(calls, *parsedCall)
						}
					}
				}
			}
		}
	}
	return calls
}

func parseFunctionCall(entry map[string]any, respID string) *types.ToolRequest {
	callID := asString(entry["call_id"])
	if callID == "" {
		callID = asString(entry["id"])
	}
	if callID == "" {
		return nil
	}
	name := asString(entry["name"])
	args := strings.TrimSpace(asString(entry["arguments"]))
	if args == "" {
		args = "{}"
	}
	return &types.ToolRequest{
		ResponseID: respID,
		ToolCallID: callID,
		Name:       name,
		Arguments:  json.RawMessage([]byte(args)),
	}
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func defaultTools() []any {
	return []any{
		map[string]any{
			"type": "web_search_preview",
		},
		map[string]any{
			"type":        "function",
			"name":        "switchbot_control_device",
			"description": "SwitchBot API を使ってデバイスを操作します。",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"device_id": map[string]any{
						"type":        "string",
						"description": "SwitchBot デバイスの ID",
					},
					"device": map[string]any{
						"type":        "string",
						"description": "環境変数 SWITCHBOT_DEVICE_MAP で定義したエイリアス名",
					},
					"command": map[string]any{
						"type":        "string",
						"description": "SwitchBot API の command (例: turnOn, turnOff, press)",
					},
					"parameter": map[string]any{
						"type":        "string",
						"description": "必要に応じて command に渡す parameter。不要なら default。",
					},
					"command_type": map[string]any{
						"type":        "string",
						"description": "SwitchBot API の commandType。省略時は command。",
					},
				},
				"required": []string{"command"},
			},
		},
		map[string]any{
			"type":        "function",
			"name":        "schedule_timer",
			"description": "指定時刻にリマインドをセットします。role=system のメッセージとして再度送ります。",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"type": map[string]any{
						"type":        "string",
						"enum":        []string{"absolute", "relative"},
						"description": "absolute または relative を指定してください。",
					},
					"description": map[string]any{
						"type":        "string",
						"description": "その時間に知らせたい内容（短め）。",
					},
					"minutes": map[string]any{
						"type":        "integer",
						"description": "relative の場合、何分後か（整数）。",
					},
					"year": map[string]any{
						"type":        "integer",
						"description": "absolute の場合の年（西暦）。",
					},
					"month": map[string]any{
						"type":        "integer",
						"description": "absolute の場合の月（1-12）。",
					},
					"day": map[string]any{
						"type":        "integer",
						"description": "absolute の場合の日（1-31）。",
					},
					"hour": map[string]any{
						"type":        "integer",
						"description": "absolute の場合の時（0-23）。",
					},
					"minute": map[string]any{
						"type":        "integer",
						"description": "absolute の場合の分（0-59）。",
					},
				},
				"required": []string{"type", "description"},
			},
		},
	}
}
