package userlocation

import (
	"context"
	"fmt"

	"github.com/tetetratra/smart-speaker/internal/states/userlocation"
)

type Tool struct {
	Store *userlocation.Store
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
		"description": "ユーザーの場所と更新日時を取得します。最後に受信した場所であり、実際の現在地を保証しません。更新日時を考慮して回答してください。未受信時やサーバー再起動後はlocationがnullになります。",
		"parameters": map[string]any{
			"type": "object", "properties": map[string]any{},
			"required": []string{}, "additionalProperties": false,
		},
	}
}
