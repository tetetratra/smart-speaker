package switchbot

import (
	"context"
	"fmt"
	"strings"

	"smart-speaker/internal/tools"
)

const hub2ToolName = "hub2_get_environment"

const hub2Alias = "hub2"

// Hub2Tool は温度/湿度/照度を取得するツールです。
type Hub2Tool struct {
	client *Client
	ctx    context.Context
}

func NewHub2(token, secret, deviceMap string) *Hub2Tool {
	client := NewSwitchbotClient(token, secret, deviceMap)
	if client == nil {
		return nil
	}
	return &Hub2Tool{client: client}
}

func (t *Hub2Tool) Name() string { return hub2ToolName }

func (t *Hub2Tool) SetContext(ctx context.Context) {
	t.ctx = ctx
}

func (t *Hub2Tool) Run(args map[string]any) (map[string]any, error) {
	if t.client == nil {
		return nil, errNotConfigured
	}
	ctx := t.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	status, err := t.client.GetStatus(ctx, "", hub2Alias)
	if err != nil {
		return nil, err
	}
	body, _ := status["body"].(map[string]any)
	temp := "取得不可"
	if body != nil {
		if v, ok := body["temperature"]; ok {
			temp = normalizeValue(v)
		}
	}
	humidity := "取得不可"
	if body != nil {
		if v, ok := body["humidity"]; ok {
			humidity = normalizeValue(v)
		}
	}
	lightLevel := "取得不可"
	if body != nil {
		if v, ok := body["lightLevel"]; ok {
			lightLevel = normalizeValue(v)
		}
	}
	return map[string]any{
		"temperature": temp,
		"humidity":    humidity,
		"light_level": lightLevel,
	}, nil
}

func (t *Hub2Tool) Definition() map[string]any {
	return map[string]any{
		"type":        "function",
		"name":        hub2ToolName,
		"description": "Hub2の温度・湿度・照度を取得します。",
		"parameters": map[string]any{
			"type":                 "object",
			"properties":           map[string]any{},
			"additionalProperties": false,
		},
	}
}

var _ tools.Handler = (*Hub2Tool)(nil)
var _ tools.ContextAware = (*Hub2Tool)(nil)
var _ tools.DefinitionProvider = (*Hub2Tool)(nil)

func normalizeValue(v any) string {
	text := strings.TrimSpace(fmt.Sprint(v))
	if text == "" || text == "<nil>" {
		return "取得不可"
	}
	return text
}
