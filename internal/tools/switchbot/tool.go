package switchbot

import (
	"context"
	"errors"
	"strings"

	"smart-speaker/internal/tools"
)

const toolName = "switchbot_control_device"

// Tool はSwitchBotのfunction toolです。
type Tool struct {
	client *Client
	ctx    context.Context
}

var errNotConfigured = errors.New("SwitchBot が設定されていません")

func New(token, secret, deviceMap string) *Tool {
	client := NewSwitchbotClient(token, secret, deviceMap)
	if client == nil {
		return nil
	}
	return &Tool{client: client}
}

func (t *Tool) Name() string { return toolName }

func (t *Tool) Run(args map[string]any) (map[string]any, error) {
	if t.client == nil {
		return nil, errNotConfigured
	}
	cmd := Command{
		DeviceAlias: strings.TrimSpace(asString(args["device"])),
		DeviceID:    strings.TrimSpace(asString(args["device_id"])),
		Command:     strings.TrimSpace(asString(args["command"])),
		Parameter:   strings.TrimSpace(asString(args["parameter"])),
		CommandType: strings.TrimSpace(asString(args["command_type"])),
	}
	ctx := t.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return t.client.Execute(ctx, cmd)
}

func (t *Tool) SetContext(ctx context.Context) {
	t.ctx = ctx
}

func (t *Tool) Definition() map[string]any {
	return map[string]any{
		"type":        "function",
		"name":        toolName,
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
	}
}

func asString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

var _ tools.Handler = (*Tool)(nil)
var _ tools.ContextAware = (*Tool)(nil)
var _ tools.DefinitionProvider = (*Tool)(nil)
