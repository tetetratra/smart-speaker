package subagent

import (
	"context"
	"fmt"
	"github.com/tetetratra/smart-speaker/internal/subagent"
	"github.com/tetetratra/smart-speaker/internal/tools"
)

type Tool struct {
	Service *subagent.Service
	ctx     context.Context
}

func (t *Tool) Name() string                   { return "ask_subagent" }
func (t *Tool) SetContext(ctx context.Context) { t.ctx = ctx }
func (t *Tool) Run(args map[string]any) (map[string]any, error) {
	if t.Service == nil {
		return nil, fmt.Errorf("サブエージェントが設定されていません")
	}
	for key := range args {
		if key != "input" && key != "session_id" {
			return nil, fmt.Errorf("未対応の引数: %s", key)
		}
	}
	input, _ := args["input"].(string)
	if value, exists := args["session_id"]; exists {
		if _, ok := value.(string); !ok {
			return nil, fmt.Errorf("session_idは文字列で指定してください")
		}
	}
	id, _ := args["session_id"].(string)
	ctx := t.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := t.Service.Call(ctx, input, id)
	if err != nil {
		return nil, err
	}
	return map[string]any{"session_id": result.SessionID, "text": result.Text}, nil
}
func (t *Tool) Definition() map[string]any {
	return tools.DefinitionWithMode(map[string]any{"type": "function", "name": t.Name(), "description": "複雑な質問・作業を高性能なテキスト専用エージェントへ依頼します。ユーザーの了承がある場合だけ呼び出してください。依頼ごとに画面の許可を待ち、許可前には送信しません。初回はsession_idを空にし、続きは返されたIDを指定します。親の会話履歴は自動で渡らないため、inputに必要な内容を記載してください。", "parameters": map[string]any{"type": "object", "properties": map[string]any{"input": map[string]any{"type": "string", "description": "送信する依頼文"}, "session_id": map[string]any{"type": "string", "description": "初回は空文字、継続時は前回のID"}}, "required": []string{"input", "session_id"}, "additionalProperties": false}}, tools.ToolModeRead)
}
