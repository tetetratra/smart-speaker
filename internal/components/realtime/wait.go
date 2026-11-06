package realtime

import (
	"context"

	"github.com/tetetratra/smart-speaker/internal/types"
)

const waitToolName = "wait_for_user"

// withWaitTool は会話制御専用のツールを追加する。一般のツール実行器には登録しない。
func withWaitTool(definitions []any) []any {
	result := make([]any, 0, len(definitions)+1)
	for _, definition := range definitions {
		if tool, ok := definition.(map[string]any); ok && tool["name"] == waitToolName {
			continue
		}
		result = append(result, definition)
	}
	return append(result, map[string]any{
		"type":        "function",
		"name":        waitToolName,
		"description": "雑音、テレビの声、独り言など、自分への依頼ではない音声に返答せず待つための会話制御です。ユーザーが待つよう明示した場合も使えます。自分への依頼が聞き取れない場合は使わず、短く聞き返してください。呼び出しの前後に相槌・説明・確認を発話しないでください。",
		"parameters": map[string]any{
			"type": "object", "properties": map[string]any{}, "required": []string{}, "additionalProperties": false,
		},
	})
}

func (s *stage) waitForUser(ctx context.Context, req types.ToolRequest) {
	// 完了を記録してAPI上の呼び出しを閉じるが、応答生成や情報取得後の表示処理は起動しない。
	s.calls[req.ToolCallID].finished = true
	const output = `{"waiting":true}`
	if s.cfg.ToolHistory != nil {
		id := s.cfg.ToolHistory.Begin(req)
		s.cfg.ToolHistory.Finish(id, types.ToolResultRecord{ToolCallID: req.ToolCallID, Name: req.Name, Output: []byte(output)})
	}
	s.record(ctx, "result/"+req.ToolCallID, types.RoleToolResult, output, req.GenerationID, waitToolName)
	s.transport.send(map[string]any{"type": "conversation.item.create", "item": map[string]any{
		"type": "function_call_output", "call_id": req.ToolCallID, "output": output,
	}})
	// wantResponseを変更しない。並行して届いた次のユーザー入力を抑制しないため。
}
