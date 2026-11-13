package execution

import (
	"context"
	"encoding/json"

	"github.com/tetetratra/smart-speaker/internal/states/toolhistory"
	"github.com/tetetratra/smart-speaker/internal/tools"
	"github.com/tetetratra/smart-speaker/internal/types"
)

// Executor は会話と予約で共有する、1回分のツール実行処理。
type Executor struct {
	handlers map[string]tools.Handler
	history  *toolhistory.Store
}

func New(handlers map[string]tools.Handler, history *toolhistory.Store) *Executor {
	return &Executor{handlers: handlers, history: history}
}

// Execute は成功と失敗を同じJSON形式で返し、画面用の履歴を記録する。
func (e *Executor) Execute(ctx context.Context, req types.ToolRequest) types.ToolResultRecord {
	args := map[string]any{}
	var argsErr error
	if len(req.Arguments) > 0 {
		argsErr = json.Unmarshal(req.Arguments, &args)
	}
	var entryID string
	if e.history != nil {
		var title string
		if provider, ok := e.handlers[req.Name].(tools.TitleProvider); ok && argsErr == nil && args != nil {
			title = provider.Title(args)
		}
		entryID = e.history.Begin(req, title)
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
	record := types.ToolResultRecord{ToolCallID: req.ToolCallID, Name: req.Name, GenerationID: req.GenerationID, Output: output}
	if e.history != nil {
		e.history.Finish(entryID, record)
	}
	return record
}
