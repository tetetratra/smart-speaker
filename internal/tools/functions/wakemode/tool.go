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
	// 呼びかけの言葉やしきい値はデプロイ時の環境変数で決めるため、必須モードだけを切り替える。
	for key := range args {
		if key != "required" {
			return nil, fmt.Errorf("未対応の引数: %s。呼びかけの言葉やしきい値は変更できません", key)
		}
	}
	required, ok := args["required"].(bool)
	if !ok {
		return nil, fmt.Errorf("requiredにtrueまたはfalseを指定してください")
	}
	mode, err := t.Store.SetRequired(required, time.Now())
	return map[string]any{"mode": mode}, err
}
func (t *Tool) Definition() map[string]any {
	props := map[string]any{}
	required := []string{}
	mode := tools.ToolModeRead
	description := "ウェイクアップワード必須モード、呼びかけの言葉と判定のしきい値、呼びかけ後の会話中かどうかを確認します。"
	if !t.ReadOnly {
		mode = tools.ToolModeWrite
		description = "ユーザーの依頼または登録済み予約ジョブに従って、ウェイクアップワード必須モードを切り替えます。required=trueでは呼びかけで始まる発話だけを受け付け、呼びかけ後は最後の発話から設定された時間（既定5分）、呼びかけなしで会話できます。falseは呼びかけ不要です。仕事中や夜間はtrue、終了時はfalseにします。呼びかけの言葉や判定のしきい値は変更できません。音声通知不要の予約では、変更後の読み上げは不要です。"
		props = map[string]any{"required": map[string]any{"type": "boolean"}}
		required = []string{"required"}
	}
	return tools.DefinitionWithMode(map[string]any{"type": "function", "name": t.Name(), "description": description, "parameters": map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}}, mode)
}
