package whiteboard

import (
	"fmt"
	"strings"

	"smart-speaker/internal/tools"
)

const toolName = "set_whiteboard"

const (
	boardDescription = `
アプリ画面に常に表示されているボードに表示する文章を設定します。
会話においてなにか情報があるときは毎回こちらのツールを呼び出し、活用するようにしてください。
ユーザーの許可を得ずに任意のタイミングで積極的に利用してください。
文の内容は5行程度を限度とします。
複雑な情報の場合、マークダウン記法を用いて構造化するようにしてください。
`
	boardContentDesc = "ボードに表示する文章。表示内容は上書きされます。"
)

// Tool は黒板表示の内容を更新します。
type Tool struct{}

func New() *Tool {
	return &Tool{}
}

func (t *Tool) Name() string { return toolName }

func (t *Tool) Run(args map[string]any) (map[string]any, error) {
	content, ok := args["content"].(string)
	if !ok {
		return nil, fmt.Errorf("content is required")
	}
	content = strings.ReplaceAll(content, `\n`, "\n")
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, fmt.Errorf("content is required")
	}
	return map[string]any{
		"content": content,
	}, nil
}

func (t *Tool) Definition() map[string]any {
	return map[string]any{
		"type":        "function",
		"name":        toolName,
		"description": boardDescription,
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"content": map[string]any{
					"type":        "string",
					"description": boardContentDesc,
				},
			},
			"required": []string{"content"},
		},
	}
}

var _ tools.Handler = (*Tool)(nil)
var _ tools.DefinitionProvider = (*Tool)(nil)
