package diary

import (
	"fmt"
	"strings"
	"time"

	"smart-speaker/internal/state"
	"smart-speaker/internal/tools"
)

const toolName = "write_diary"

const (
	diaryDescription = `直近の会話を日記として保存します。
指示がない限り勝手に呼び出さないでください。
`

	diaryContentDescription = `日記本文。文章の構造化などはせずに以下の話題などに触れて1〜5行前後で書いてください。
- 会話した話題とその詳細
- 会話を経て知った、ユーザーの好みや傾向、性格など
- 会話を経て知った、一般的な事柄に対する知識や理解など
会話が短かった場合や、特に日記に書くべき内容がない場合は、会話した内容のみを簡潔に書いてください。
また、句点ごとに改行してください。
`
)

// Tool は会話の日記をファイルに保存します。
type Tool struct{}

func New() *Tool {
	return &Tool{}
}

func (t *Tool) Name() string { return toolName }

func (t *Tool) Run(args map[string]any) (map[string]any, error) {
	content := toStr(args["content"])
	content = strings.ReplaceAll(content, `\n`, "\n")
	if strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("content is required")
	}
	when := state.GetLastActivityAt()
	if when.IsZero() {
		when = time.Now()
	}
	path, err := state.AppendDiaryEntry(when, content)
	if err != nil {
		return nil, fmt.Errorf("failed to write diary: %w", err)
	}
	return map[string]any{
		"path":      path,
		"timestamp": when.Format(time.RFC3339),
	}, nil
}

func (t *Tool) Definition() map[string]any {
	return map[string]any{
		"type":        "function",
		"name":        toolName,
		"description": diaryDescription,
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"content": map[string]any{
					"type":        "string",
					"description": diaryContentDescription,
				},
			},
			"required": []string{"content"},
		},
	}
}

func toStr(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

var _ tools.Handler = (*Tool)(nil)
var _ tools.DefinitionProvider = (*Tool)(nil)
