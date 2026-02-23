package switchbot

import (
	"context"
	"fmt"
	"strings"

	"smart-speaker/internal/tools"
)

const blindToolName = "blind_control"

const blindAlias = "ブラインド"

// BlindTiltTool はブラインドの開閉を行う簡易ツールです。
type BlindTiltTool struct {
	client *Client
	ctx    context.Context
}

func NewBlindTilt(token, secret, deviceMap string) *BlindTiltTool {
	client := NewSwitchbotClient(token, secret, deviceMap)
	if client == nil {
		return nil
	}
	return &BlindTiltTool{client: client}
}

func (t *BlindTiltTool) Name() string { return blindToolName }

func (t *BlindTiltTool) SetContext(ctx context.Context) {
	t.ctx = ctx
}

func (t *BlindTiltTool) Run(args map[string]any) (map[string]any, error) {
	if t.client == nil {
		return nil, errNotConfigured
	}
	action := strings.TrimSpace(asString(args["action"]))
	cmd, ok := blindActionCommand(action)
	if !ok {
		return nil, fmt.Errorf("action must be one of open, close")
	}
	ctx := t.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return t.client.Execute(ctx, cmd)
}

func (t *BlindTiltTool) Definition() map[string]any {
	return map[string]any{
		"type":        "function",
		"name":        blindToolName,
		"description": "ブラインド(Blind Tilt)を開閉します。ユーザーが明示的に操作の要求をしたときのみ呼び出すこと。",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"enum":        []string{"open", "close"},
					"description": "open / close",
				},
			},
			"required": []string{"action"},
		},
	}
}

func blindActionCommand(action string) (Command, bool) {
	switch action {
	case "open":
		return Command{
			DeviceAlias: blindAlias,
			Command:     "fullyOpen",
			CommandType: "command",
			Parameter:   "default",
		}, true
	case "close":
		return Command{
			DeviceAlias: blindAlias,
			Command:     "closeDown",
			CommandType: "command",
			Parameter:   "default",
		}, true
	default:
		return Command{}, false
	}
}

var _ tools.Handler = (*BlindTiltTool)(nil)
var _ tools.ContextAware = (*BlindTiltTool)(nil)
var _ tools.DefinitionProvider = (*BlindTiltTool)(nil)
