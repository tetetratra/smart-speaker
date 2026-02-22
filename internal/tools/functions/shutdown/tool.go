package shutdown

import "smart-speaker/internal/tools"

const toolName = "shutdown_mode"

// Tool はフロントエンドにシャットダウンモード移行を指示します。
type Tool struct{}

func New() *Tool {
	return &Tool{}
}

func (t *Tool) Name() string { return toolName }

func (t *Tool) Run(args map[string]any) (map[string]any, error) {
	return map[string]any{
		"shutdown_mode": true,
	}, nil
}

func (t *Tool) Definition() map[string]any {
	return map[string]any{
		"type":        "function",
		"name":        toolName,
		"description": "ユーザーが明示的に休止/停止を求めたときのみ呼び出します。呼び出すとフロント側は音声入力を停止し、ウェイクワードで復帰します。",
		"parameters": map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}
}

var _ tools.Handler = (*Tool)(nil)
var _ tools.DefinitionProvider = (*Tool)(nil)
