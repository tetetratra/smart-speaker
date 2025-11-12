package tools

import (
	"context"
	"strings"
	"sync"
	"time"

	"smart-speaker/internal/tools/switchbot"
)

// Executor routes function calling requests to concrete implementations.
type Executor struct {
	switchOnce sync.Once
	switchBot  *switchbot.Client
	switchErr  error
}

// NewExecutor creates a new tool executor.
func NewExecutor() *Executor {
	return &Executor{}
}

// Execute dispatches to a tool by name and returns the result payload.
func (e *Executor) Execute(ctx context.Context, name string, args map[string]any) map[string]any {
	switch name {
	case "get_current_time":
		return runCurrentTimeTool(args)
	case "get_weather":
		return runWeatherTool(args)
	case "switchbot_control_device":
		return e.runSwitchBotTool(ctx, args)
	default:
		return map[string]any{
			"error": "unknown function: " + name,
		}
	}
}

func runCurrentTimeTool(args map[string]any) map[string]any {
	if hasArguments(args) {
		return map[string]any{"error": "get_current_time は引数を受け付けません"}
	}
	now := time.Now()
	return map[string]any{
		"iso8601":  now.Format(time.RFC3339),
		"timezone": now.Location().String(),
	}
}

func runWeatherTool(args map[string]any) map[string]any {
	if hasArguments(args) {
		return map[string]any{"error": "get_weather は引数を受け付けません"}
	}
	time.Sleep(5 * time.Second)
	return map[string]any{
		"forecast":    "晴れ",
		"temperature": 23.5,
	}
}

func (e *Executor) runSwitchBotTool(ctx context.Context, args map[string]any) map[string]any {
	client, err := e.ensureSwitchBotClient()
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	command := switchbot.Command{
		DeviceAlias: strings.TrimSpace(asString(args["device"])),
		DeviceID:    strings.TrimSpace(asString(args["device_id"])),
		Command:     strings.TrimSpace(asString(args["command"])),
		Parameter:   strings.TrimSpace(asString(args["parameter"])),
		CommandType: strings.TrimSpace(asString(args["command_type"])),
	}
	result, execErr := client.Execute(ctx, command)
	if execErr != nil {
		return map[string]any{"error": execErr.Error()}
	}
	return result
}

func (e *Executor) ensureSwitchBotClient() (*switchbot.Client, error) {
	e.switchOnce.Do(func() {
		e.switchBot, e.switchErr = switchbot.NewFromEnv()
	})
	return e.switchBot, e.switchErr
}

func hasArguments(args map[string]any) bool {
	if len(args) == 0 {
		return false
	}
	for _, v := range args {
		if v == nil {
			continue
		}
		if str, ok := v.(string); ok {
			if strings.TrimSpace(str) == "" {
				continue
			}
		}
		return true
	}
	return false
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
