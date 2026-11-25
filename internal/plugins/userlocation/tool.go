package userlocation

import (
	"context"
	"fmt"
)

// Tool は、最後に受け取った場所を返す AI ツール get_current_location。
type Tool struct {
	Store *Store
}

func (t *Tool) Name() string { return "get_current_location" }

func (t *Tool) Run(_ context.Context, args map[string]any) (map[string]any, error) {
	if len(args) > 0 {
		return nil, fmt.Errorf("引数は指定しません")
	}
	return map[string]any{"location": t.Store.Snapshot()}, nil
}

func (t *Tool) Definition() map[string]any {
	return map[string]any{
		"type": "function", "name": t.Name(),
		"description": "システムが把握しているユーザーの居場所と更新日時を取得します。最後に受信した場所を現在の状態として保持しています。同じ場所の更新では会話へのイベントは発行されないため、新しいイベントがないことやイベントの古さだけを理由に不明としないでください。その場でGPSを測定するツールではありません。取得結果は過去のイベントより優先してください。未受信時やサーバー再起動後はlocationがnullになります。",
		"parameters": map[string]any{
			"type": "object", "properties": map[string]any{},
			"required": []string{}, "additionalProperties": false,
		},
	}
}
