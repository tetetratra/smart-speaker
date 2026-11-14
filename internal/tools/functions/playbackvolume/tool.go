package playbackvolume

import (
	"context"
	"fmt"

	"github.com/tetetratra/smart-speaker/internal/states/playbackvolume"
)

// Toolは接続中の全端末の再生音量を一律に切り替える。
type Tool struct {
	Store *playbackvolume.Store
}

func (t *Tool) Name() string { return "set_playback_volume" }

func (t *Tool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	for key := range args {
		if key != "level" {
			return nil, fmt.Errorf("未対応の引数: %s", key)
		}
	}
	level, _ := args["level"].(string)
	var previous, current string
	switch level {
	case "up":
		previous, current = t.Store.Step(1)
	case "down":
		previous, current = t.Store.Step(-1)
	default:
		var ok bool
		if previous, ok = t.Store.Set(level); !ok {
			return nil, fmt.Errorf("levelにはmute・quiet・low・normal・boost・up・downのいずれかを指定してください")
		}
		current = level
	}
	return map[string]any{"previous": previous, "level": current}, nil
}

func (t *Tool) Definition() map[string]any {
	levels := append(append([]string{}, playbackvolume.Levels...), "up", "down")
	return map[string]any{
		"type": "function",
		"name": t.Name(),
		"description": "接続中のすべての端末の再生音量を一律に切り替えます。" +
			"ユーザーから音量の変更をはっきり頼まれたときだけ使ってください。周囲の状況や会話の内容から判断して、頼まれていないのに音量を変えてはいけません。" +
			"levelは小さい順にmute（0倍・無音）、quiet（0.3倍）、low（0.6倍）、normal（1倍）、boost（1.5倍・最大）です。" +
			"「上げて」「下げて」のように段階を言われなかった場合は、upかdownで1段階だけ動かします。端ではそれ以上動きません。",
		"parameters": map[string]any{
			"type":                 "object",
			"properties":           map[string]any{"level": map[string]any{"type": "string", "enum": levels}},
			"required":             []string{"level"},
			"additionalProperties": false,
		},
	}
}
