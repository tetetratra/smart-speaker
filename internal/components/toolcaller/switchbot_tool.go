package toolcaller

import (
	"context"
	"errors"
	"strings"

	switchbot "smart-speaker/internal/tools/switchbot"
)

const switchBotToolName = "switchbot_control_device"

type SwitchBotTool struct {
	client *switchbot.Client
	ctx    context.Context
}

var errSwitchBotNotConfigured = errors.New("SwitchBot が設定されていません")

func NewSwitchBotTool(token, secret, deviceMap string) *SwitchBotTool {
	client := switchbot.NewSwitchbotClient(token, secret, deviceMap)
	return &SwitchBotTool{client: client}
}

func (t *SwitchBotTool) Name() string {
	return switchBotToolName
}

func (t *SwitchBotTool) Run(args map[string]any) (map[string]any, error) {
	if t.client == nil {
		return nil, errSwitchBotNotConfigured
	}
	cmd := switchbot.Command{
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

func (t *SwitchBotTool) SetContext(ctx context.Context) {
	t.ctx = ctx
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
