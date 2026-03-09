package volume

import (
	"fmt"

	"smart-speaker/internal/tools"
)

const toolName = "set_volume"

// Tool は再生音量の変更をフロントへ指示します。
type Tool struct{}

func New() *Tool {
	return &Tool{}
}

func (t *Tool) Name() string { return toolName }

func (t *Tool) Run(args map[string]any) (map[string]any, error) {
	volume, ok := toInt(args["volume_percent"])
	if !ok {
		return nil, fmt.Errorf("volume_percent must be an integer between 1 and 100")
	}
	return map[string]any{
		"volume_percent": volume,
	}, nil
}

func (t *Tool) Definition() map[string]any {
	return map[string]any{
		"type":        "function",
		"name":        toolName,
		"description": "ユーザーが音量変更を明示的に求めたときのみ呼び出します。1から100までの整数を volume_percent に指定します。",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"volume_percent": map[string]any{
					"type":        "integer",
					"minimum":     1,
					"maximum":     100,
					"description": "再生音量のパーセント。1から100の整数。",
				},
			},
			"required": []string{"volume_percent"},
		},
	}
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return validateVolumePercent(n)
	case int32:
		return validateVolumePercent(int(n))
	case int64:
		return validateVolumePercent(int(n))
	case float64:
		if n != float64(int(n)) {
			return 0, false
		}
		return validateVolumePercent(int(n))
	default:
		return 0, false
	}
}

func validateVolumePercent(volume int) (int, bool) {
	if volume < 1 || volume > 100 {
		return 0, false
	}
	return volume, true
}

var _ tools.Handler = (*Tool)(nil)
var _ tools.DefinitionProvider = (*Tool)(nil)
