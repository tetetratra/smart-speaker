package switchbot

import (
	"context"
	"fmt"
	"strings"

	"smart-speaker/internal/tools"
)

const lightToolName = "light_control"

const lightAlias = "ライト"

// LightTool は照明専用の簡易ツールです。
type LightTool struct {
	client *Client
	ctx    context.Context
}

func NewLight(token, secret, deviceMap string) *LightTool {
	client := NewSwitchbotClient(token, secret, deviceMap)
	if client == nil {
		return nil
	}
	return &LightTool{client: client}
}

func (t *LightTool) Name() string { return lightToolName }

func (t *LightTool) SetContext(ctx context.Context) {
	t.ctx = ctx
}

func (t *LightTool) Run(args map[string]any) (map[string]any, error) {
	if t.client == nil {
		return nil, errNotConfigured
	}
	action := strings.TrimSpace(asString(args["action"]))
	cmd, ok := lightActionCommand(action)
	if !ok {
		return nil, fmt.Errorf("action must be one of strong_on, on, weak_on, off")
	}
	ctx := t.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return t.client.Execute(ctx, cmd)
}

func (t *LightTool) Definition() map[string]any {
	return map[string]any{
		"type":        "function",
		"name":        lightToolName,
		"description": "ライトを操作します。明るさは強/中/弱の3段階。ユーザーが明示的に操作の要求をしたときのみ呼び出すこと。",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"enum":        []string{"strong_on", "on", "weak_on", "off"},
					"description": "strong_on / on / weak_on / off",
				},
			},
			"required": []string{"action"},
		},
	}
}

func lightActionCommand(action string) (Command, bool) {
	switch action {
	case "strong_on":
		return Command{
			DeviceAlias: lightAlias,
			Command:     "setBrightness",
			CommandType: "command",
			Parameter:   "100",
		}, true
	case "on":
		return Command{
			DeviceAlias: lightAlias,
			Command:     "setBrightness",
			CommandType: "command",
			Parameter:   "50",
		}, true
	case "weak_on":
		return Command{
			DeviceAlias: lightAlias,
			Command:     "setBrightness",
			CommandType: "command",
			Parameter:   "10",
		}, true
	case "off":
		return Command{
			DeviceAlias: lightAlias,
			Command:     "turnOff",
			CommandType: "command",
			Parameter:   "default",
		}, true
	default:
		return Command{}, false
	}
}

var _ tools.Handler = (*LightTool)(nil)
var _ tools.ContextAware = (*LightTool)(nil)
var _ tools.DefinitionProvider = (*LightTool)(nil)
