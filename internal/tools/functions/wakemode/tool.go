package wakemode

import (
	"fmt"
	"github.com/tetetratra/smart-speaker/internal/states/wakemode"
	"github.com/tetetratra/smart-speaker/internal/tools"
	"math"
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
		if key != "required" && key != "words" && key != "max_prefix" && key != "min_similarity" && key != "min_confidence" {
			return nil, fmt.Errorf("未対応の引数: %s", key)
		}
	}
	required, ok := args["required"].(bool)
	if !ok {
		return nil, fmt.Errorf("requiredにtrueまたはfalseを指定してください")
	}
	var options wakemode.Options
	if value, exists := args["words"]; exists {
		list, ok := value.([]any)
		if !ok || len(list) == 0 {
			return nil, fmt.Errorf("wordsは1件以上の文字列の配列で指定してください")
		}
		for _, item := range list {
			word, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("wordsは文字列の配列で指定してください")
			}
			options.Words = append(options.Words, word)
		}
	}
	if value, exists := args["max_prefix"]; exists {
		n, ok := value.(float64)
		if !ok || n != math.Trunc(n) {
			return nil, fmt.Errorf("max_prefixは整数で指定してください")
		}
		prefix := int(n)
		options.MaxPrefix = &prefix
	}
	for key, target := range map[string]**float64{"min_similarity": &options.MinSimilarity, "min_confidence": &options.MinConfidence} {
		if value, exists := args[key]; exists {
			n, ok := value.(float64)
			if !ok {
				return nil, fmt.Errorf("%sは数値で指定してください", key)
			}
			*target = &n
		}
	}
	mode, err := t.Store.SetOptions(required, options, time.Now())
	return map[string]any{"mode": mode}, err
}
func (t *Tool) Definition() map[string]any {
	props := map[string]any{}
	required := []string{}
	mode := tools.ToolModeRead
	description := "ウェイクアップワード必須モード、呼びかけの言葉と判定のしきい値、呼びかけ後の会話中かどうかを確認します。"
	if !t.ReadOnly {
		mode = tools.ToolModeWrite
		description = "ユーザーの依頼または登録済み予約ジョブに従って、ウェイクアップワード必須モードと呼びかけの判定を設定します。required=trueでは呼びかけで始まる発話だけを受け付け、呼びかけ後は最後の発話から1分間、呼びかけなしで会話できます。falseは呼びかけ不要です。仕事中や夜間はtrue、終了時はfalseにします。ほかの引数は変更するときだけ指定します。wordsは呼びかけの言葉の配列、max_prefixは呼びかけの前に許す文字数（0〜20）、min_similarityは言葉との類似度の下限（0より大きく1以下）、min_confidenceは音声認識の信頼度の下限（0より大きく1以下）です。音声通知不要の予約では、変更後の読み上げは不要です。"
		props = map[string]any{"required": map[string]any{"type": "boolean"}, "words": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "max_prefix": map[string]any{"type": "integer"}, "min_similarity": map[string]any{"type": "number"}, "min_confidence": map[string]any{"type": "number"}}
		required = []string{"required"}
	}
	return tools.DefinitionWithMode(map[string]any{"type": "function", "name": t.Name(), "description": description, "parameters": map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}}, mode)
}
