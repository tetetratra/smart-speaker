package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/google/uuid"
	"github.com/tetetratra/smart-speaker/internal/automations"
	"github.com/tetetratra/smart-speaker/internal/automations/script"
	"github.com/tetetratra/smart-speaker/internal/states/toolhistory"
	"github.com/tetetratra/smart-speaker/internal/tools"
	"github.com/tetetratra/smart-speaker/internal/types"
)

// automationRunner は発火した予約ジョブのスクリプトを実行する。
type automationRunner struct {
	scheduled chan<- types.ScheduledPrompt
	direct    chan<- types.DirectToolCall
	history   *toolhistory.Store
	// handlers はツールの登録後に buildStages で設定する。予約の実行開始より前に設定される。
	handlers map[string]tools.Handler
}

func (r *automationRunner) run(ctx context.Context, delivery automations.Delivery) error {
	reminds := 0
	return script.Run(ctx, delivery.Script, script.Funcs{
		Remind: func(ctx context.Context, text string) error {
			reminds++
			// 会話側は同じIDの予約を上書きするため、1回のスクリプトで複数回呼ばれても消えないよう番号を付ける。
			id := delivery.ID
			if reminds > 1 {
				id = fmt.Sprintf("%s/%d", delivery.ID, reminds)
			}
			prompt := types.ScheduledPrompt{ID: id, Name: delivery.Name, Instruction: text, Speak: true, Source: "automation/" + delivery.ID}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case r.scheduled <- prompt:
				return nil
			}
		},
		ToolCall: func(ctx context.Context, name string, args map[string]any) (json.RawMessage, error) {
			return r.toolCall(ctx, delivery, name, args)
		},
	}, script.DefaultTimeout)
}

// toolCall はAIを通さずにツールを実行し、呼び出しと結果を会話履歴へ送る。
func (r *automationRunner) toolCall(ctx context.Context, delivery automations.Delivery, name string, args map[string]any) (json.RawMessage, error) {
	handler, ok := r.handlers[name]
	if !ok {
		return nil, fmt.Errorf("ツールがありません: %s", name)
	}
	arguments, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	req := types.ToolRequest{ToolCallID: "automation_" + uuid.NewString(), Name: name, Arguments: arguments}
	var entryID string
	if r.history != nil {
		entryID = r.history.Begin(req)
	}
	// 会話中のAIが受け取るのと同じ形にそろえ、ツールの失敗はerrorを持つ結果として返す。
	result, err := handler.Run(args)
	if err != nil {
		result = map[string]any{"error": err.Error()}
	}
	output, err := json.Marshal(result)
	if err != nil {
		output = []byte(`{"error":"result encoding failed"}`)
	}
	record := types.ToolResultRecord{ToolCallID: req.ToolCallID, Name: name, Output: output}
	if r.history != nil {
		r.history.Finish(entryID, record)
	}
	select {
	case r.direct <- types.DirectToolCall{Source: "automation/" + delivery.ID, Request: req, Result: record}:
	case <-ctx.Done():
		log.Printf("予約ジョブのツール呼び出しを会話履歴に残せませんでした job=%s tool=%s: %v", delivery.ID, name, ctx.Err())
	}
	return output, nil
}
