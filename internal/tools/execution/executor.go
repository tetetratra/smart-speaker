package execution

import (
	"context"
	"encoding/json"

	"github.com/tetetratra/smart-speaker/internal/tools"
	"github.com/tetetratra/smart-speaker/internal/types"
)

// Executor は会話と予約で共有する、1回分のツール実行処理。
type Executor struct {
	handlers map[string]tools.Handler
}

func New(handlers map[string]tools.Handler) *Executor {
	return &Executor{handlers: handlers}
}

// Title は、画面に出すツールのタイトルを引数から作る。タイトルを持たないツールや引数が壊れているときは空。
func (e *Executor) Title(name string, arguments json.RawMessage) string {
	provider, ok := e.handlers[name].(tools.TitleProvider)
	if !ok {
		return ""
	}
	args := map[string]any{}
	if err := json.Unmarshal(arguments, &args); err != nil || args == nil {
		return ""
	}
	return provider.Title(args)
}

// Execute は成功と失敗を同じJSON形式で返す。
func (e *Executor) Execute(ctx context.Context, req types.ToolRequest) types.ToolResultRecord {
	args := map[string]any{}
	var argsErr error
	if len(req.Arguments) > 0 {
		argsErr = json.Unmarshal(req.Arguments, &args)
	}
	var result map[string]any
	if err := ctx.Err(); err != nil {
		result = map[string]any{"error": err.Error()}
	} else if argsErr != nil || args == nil {
		result = map[string]any{"error": "invalid tool arguments"}
	} else if handler, ok := e.handlers[req.Name]; !ok {
		result = map[string]any{"error": "unknown function: " + req.Name}
	} else {
		out, err := handler.Run(ctx, args)
		if err != nil {
			result = map[string]any{"error": err.Error()}
		} else {
			result = out
		}
	}
	output, err := json.Marshal(result)
	if err != nil {
		output = []byte(`{"error":"result encoding failed"}`)
	}
	return types.ToolResultRecord{ToolCallID: req.ToolCallID, Name: req.Name, GenerationID: req.GenerationID, Output: output}
}
