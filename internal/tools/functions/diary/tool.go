package diary

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"smart-speaker/internal/tools"
)

const toolName = "write_diary"

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
	timestamp := toStr(args["timestamp"])
	when := time.Now()
	if strings.TrimSpace(timestamp) != "" {
		if parsed, err := time.Parse(time.RFC3339, timestamp); err == nil {
			when = parsed
		}
	}
	if err := os.MkdirAll(filepath.Join("tmp", "diary"), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create diary dir: %w", err)
	}
	filename := when.Format("2006-01-02_150405") + ".md"
	path := filepath.Join("tmp", "diary", filename)
	if err := os.WriteFile(path, []byte(content+"\n"), 0o644); err != nil {
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
		"description": "直近の会話を日記として保存します。必ず指定のフォーマットで各セクション3〜5行程度で書き、会話ごとに1ファイルにしてください。",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"content": map[string]any{
					"type":        "string",
					"description": "日記本文。以下の形式を厳守してください。\n# (会話の日時)\n## 会話した話題\n...\n## ユーザーに関する知識\n...\n## 会話で得た知識\n...\n## 備考\n...\n## 今回の会話に対するあなたの感想\n...",
				},
				"timestamp": map[string]any{
					"type":        "string",
					"description": "会話の日時（RFC3339推奨）。未指定の場合は現在時刻を使います。",
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
