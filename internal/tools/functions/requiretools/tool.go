package requiretools

import "smart-speaker/internal/tools"

const toolName = "require_tools"

const toolDescription = `SwitchBotの家電操作や、タイマー設定、Googleカレンダー連携の操作をしたいときに呼び出してください。操作方法を提供します。
利用できるツールは次の通りです。
- switchbot_control_device: 照明やエアコンなどのSwitchBotデバイス操作
- schedule_timer: 指定時刻に通知するタイマー設定
- web_search: 最新情報の検索（これは常に使えます）
- google_calendar: Googleカレンダーの予定参照・作成（連携が有効な場合のみ）
このツールの出力は内部向けです。ユーザーには読み上げないでください。`

// Tool はツール一覧の要求を処理します。
type Tool struct{}

func New() *Tool {
	return &Tool{}
}

func (t *Tool) Name() string { return toolName }

func (t *Tool) Run(args map[string]any) (map[string]any, error) {
	return map[string]any{
		"message": "こちらがツール一覧です、必要に応じてツールを呼んでください。",
	}, nil
}

func (t *Tool) Definition() map[string]any {
	return map[string]any{
		"type":        "function",
		"name":        toolName,
		"description": toolDescription,
		"parameters": map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}
}

var _ tools.Handler = (*Tool)(nil)
var _ tools.DefinitionProvider = (*Tool)(nil)
