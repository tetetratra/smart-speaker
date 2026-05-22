package whiteboard

import (
	"fmt"
	"strings"

	"smart-speaker/internal/tools"
	types "smart-speaker/internal/types"
)

const toolName = "set_whiteboard"

const (
	toolDescription = `アプリ画面のホワイトボード表示へ情報を追記します。
予定・注意事項・要点など、口頭だけでは伝わりにくい情報を画面に残したいときに使ってください。
また、カレンダー情報や部屋の情報の取得などのGET系のツールを使った場合もこのツールを呼び出して情報を残してください。
返答や感想ではなく、表示用の簡潔な内容だけを書いてください。
`
	contentDescription = `ホワイトボードに追記する文章。7行程度を目安にし、URLやリンク付きテキストは含めないでください。`
)

type Tool struct {
	emit func(types.Event)
}

func New() *Tool {
	return &Tool{}
}

func (t *Tool) Name() string { return toolName }

func (t *Tool) SetEventEmitter(emit func(types.Event)) {
	t.emit = emit
}

func (t *Tool) Run(args map[string]any) (map[string]any, error) {
	content := strings.TrimSpace(asString(args["content"]))
	content = strings.ReplaceAll(content, `\n`, "\n")
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, fmt.Errorf("content is required")
	}
	if t.emit != nil {
		t.emit(types.Event{
			Kind: types.EventWhiteboardUpdate,
			Payload: types.WhiteboardUpdate{
				Content: content,
			},
		})
	}
	return map[string]any{
		"content": content,
		"updated": true,
	}, nil
}

func (t *Tool) Definition() map[string]any {
	return map[string]any{
		"type":        "function",
		"name":        toolName,
		"description": toolDescription,
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"content": map[string]any{
					"type":        "string",
					"description": contentDescription,
				},
			},
			"required":             []string{"content"},
			"additionalProperties": false,
		},
	}
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

var _ tools.Handler = (*Tool)(nil)
var _ tools.EventEmitterAware = (*Tool)(nil)
var _ tools.DefinitionProvider = (*Tool)(nil)
