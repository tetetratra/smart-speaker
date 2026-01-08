package diary

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"smart-speaker/internal/state"
	"smart-speaker/internal/tools"
)

const toolName = "write_diary"

const (
	diaryDescription = `直近の会話を日記として保存します。
指示がない限り勝手に呼び出さないでください。
必ず指定のフォーマットで各セクション5行前後で書いてください。
`

	diaryContentDescription = `日記本文。文章の構造化などはせずに以下の話題などに触れて5行前後で書いてください。
- 会話した話題
- 会話で知った、ユーザーや、一般的な事柄に対する知識
- あなたの感想
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
	if strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("content is required")
	}
	when := state.GetLastActivityAt()
	if when.IsZero() {
		when = time.Now()
	}
	if err := os.MkdirAll(filepath.Join("tmp", "diary"), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create diary dir: %w", err)
	}
	filename := when.Format("2006-01-02_150405") + ".md"
	path := filepath.Join("tmp", "diary", filename)
	header := "# " + when.Format("2006-01-02 15:04")
	body := header + "\n" + strings.TrimLeft(content, "\n") + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
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
