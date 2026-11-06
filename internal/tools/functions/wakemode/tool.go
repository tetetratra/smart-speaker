package wakemode

import (
	"fmt"
	"github.com/tetetratra/smart-speaker/internal/states/wakemode"
	"github.com/tetetratra/smart-speaker/internal/tools"
	"time"
)

type Tool struct {
	Store    *wakemode.Store
	ReadOnly bool
}

func (t *Tool) Name() string {
	if t.ReadOnly {
		return "get_wake_mode"
	}
	return "set_wake_mode"
}
func (t *Tool) Run(args map[string]any) (map[string]any, error) {
	if t.ReadOnly {
		if len(args) > 0 {
			return nil, fmt.Errorf("引数は指定しません")
		}
		return map[string]any{"mode": t.Store.Snapshot(time.Now())}, nil
	}
	for key := range args {
		if key != "required" && key != "word" {
			return nil, fmt.Errorf("未対応の引数: %s", key)
		}
	}
	required, ok := args["required"].(bool)
	if !ok {
		return nil, fmt.Errorf("requiredにtrueまたはfalseを指定してください")
	}
	word := ""
	if value, exists := args["word"]; exists {
		var ok bool
		word, ok = value.(string)
		if !ok {
			return nil, fmt.Errorf("wordは文字列で指定してください")
		}
	}
	mode, err := t.Store.Set(required, word, time.Now())
	return map[string]any{"mode": mode}, err
}
func (t *Tool) Definition() map[string]any {
	props := map[string]any{}
	required := []string{}
	mode := tools.ToolModeRead
	description := "ウェイクアップワード必須モードと呼びかけの言葉を確認します。"
	if !t.ReadOnly {
		mode = tools.ToolModeWrite
		description = "ユーザーの依頼または登録済み予約ジョブに従って、ウェイクアップワード必須モードを切り替えます。required=trueでは毎回の発話に呼びかけが必要で、呼びかけなしの発話は無視します。falseは不要です。仕事中や夜間はtrue、終了時はfalseにします。wordは呼びかけの言葉を変更するときだけ指定します。音声通知不要の予約では、変更後の読み上げは不要です。"
		props = map[string]any{"required": map[string]any{"type": "boolean"}, "word": map[string]any{"type": "string"}}
		required = []string{"required"}
	}
	return tools.DefinitionWithMode(map[string]any{"type": "function", "name": t.Name(), "description": description, "parameters": map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}}, mode)
}
