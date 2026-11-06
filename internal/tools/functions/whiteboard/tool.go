package whiteboard

import (
	"fmt"
	"strings"

	"github.com/tetetratra/smart-speaker/internal/tools"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

const toolName = "set_whiteboard"

const (
	toolDescription = `アプリ画面のホワイトボード表示へ情報を追記します。
予定・注意事項・要点など、口頭だけでは伝わりにくい情報を画面に残したいときに使ってください。
「ここをもっと詳しく書いて」などの追加表示にも使えます。既存の取得結果で足りる場合は、その情報を使って説明を追加してください。
情報取得ツールの結果も、画面に残す必要があれば口頭説明より先にこのツールで要点を残してください。
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
	return tools.DefinitionWithResponsePolicy(tools.DefinitionWithMode(map[string]any{
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
	}, tools.ToolModeWrite), tools.ResponsePolicy{
		OnSuccess:    true,
		Instructions: "ホワイトボード更新の結果です。成功した場合、表示完了そのものの報告や同じ内容の再更新は不要です。まだ説明していない取得結果があれば、その要点を短く説明してください。既に説明済み、または表示だけの依頼ならwait_for_userで待機してください。失敗時はその旨を短く伝え、同じ更新を自動で繰り返さないでください。",
	})
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
