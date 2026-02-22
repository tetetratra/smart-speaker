package volume

import (
	"fmt"
	"strings"

	"smart-speaker/internal/tools"
)

const toolName = "set_volume_preset"

// Tool は再生音量のプリセット変更をフロントへ指示します。
type Tool struct{}

func New() *Tool {
	return &Tool{}
}

func (t *Tool) Name() string { return toolName }

func (t *Tool) Run(args map[string]any) (map[string]any, error) {
	preset := strings.TrimSpace(toStr(args["preset"]))
	volume, ok := presetToPercent(preset)
	if !ok {
		return nil, fmt.Errorf("preset must be one of: small, normal, large")
	}
	return map[string]any{
		"preset":         preset,
		"volume_percent": volume,
	}, nil
}

func (t *Tool) Definition() map[string]any {
	return map[string]any{
		"type":        "function",
		"name":        toolName,
		"description": "ユーザーが音量変更を明示的に求めたときのみ呼び出します。presetは small / normal / large のみです。",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"preset": map[string]any{
					"type":        "string",
					"enum":        []string{"small", "normal", "large"},
					"description": "small=小さめ, normal=普通, large=大きめ",
				},
			},
			"required": []string{"preset"},
		},
	}
}

func toStr(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func presetToPercent(preset string) (int, bool) {
	switch preset {
	case "small":
		return 40, true
	case "normal":
		return 70, true
	case "large":
		return 100, true
	default:
		return 0, false
	}
}

var _ tools.Handler = (*Tool)(nil)
var _ tools.DefinitionProvider = (*Tool)(nil)
