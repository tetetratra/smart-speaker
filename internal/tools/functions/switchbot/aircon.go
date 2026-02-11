package switchbot

import (
	"context"
	"fmt"
	"strings"

	"smart-speaker/internal/tools"
)

const airconToolName = "aircon_control"

const airconAlias = "エアコン"

// AirconTool はエアコン専用の簡易ツールです。
type AirconTool struct {
	client *Client
	ctx    context.Context
}

func NewAircon(token, secret, deviceMap string) *AirconTool {
	client := NewSwitchbotClient(token, secret, deviceMap)
	if client == nil {
		return nil
	}
	return &AirconTool{client: client}
}

func (t *AirconTool) Name() string { return airconToolName }

func (t *AirconTool) SetContext(ctx context.Context) {
	t.ctx = ctx
}

func (t *AirconTool) Run(args map[string]any) (map[string]any, error) {
	if t.client == nil {
		return nil, errNotConfigured
	}
	action := strings.TrimSpace(asString(args["action"]))
	cmd, ok := airconActionCommand(action)
	if !ok {
		return nil, fmt.Errorf("action must be one of heat_on, cool_on, off")
	}
	ctx := t.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return t.client.Execute(ctx, cmd)
}

func (t *AirconTool) Definition() map[string]any {
	return map[string]any{
		"type":        "function",
		"name":        airconToolName,
		"description": "エアコンを操作します。27度固定の暖房/冷房/オフのみ対応。",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"enum":        []string{"heat_on", "cool_on", "off"},
					"description": "heat_on / cool_on / off",
				},
			},
			"required": []string{"action"},
		},
	}
}

func airconActionCommand(action string) (Command, bool) {
	switch action {
	case "heat_on":
		return Command{
			DeviceAlias: airconAlias,
			Command:     "setAll",
			CommandType: "command",
			Parameter:   "27,5,3,on",
		}, true
	case "cool_on":
		return Command{
			DeviceAlias: airconAlias,
			Command:     "setAll",
			CommandType: "command",
			Parameter:   "27,2,3,on",
		}, true
	case "off":
		return Command{
			DeviceAlias: airconAlias,
			Command:     "setAll",
			CommandType: "command",
			Parameter:   "27,1,3,off",
		}, true
	default:
		return Command{}, false
	}
}

var _ tools.Handler = (*AirconTool)(nil)
var _ tools.ContextAware = (*AirconTool)(nil)
var _ tools.DefinitionProvider = (*AirconTool)(nil)
