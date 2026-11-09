package main

import (
	"context"
	"fmt"

	"github.com/tetetratra/smart-speaker/internal/automations"
	"github.com/tetetratra/smart-speaker/internal/automations/script"
	"github.com/tetetratra/smart-speaker/internal/types"
)

// automationRunner は発火した予約ジョブのスクリプトを実行する。
type automationRunner struct {
	scheduled chan<- types.ScheduledPrompt
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
	}, script.DefaultTimeout)
}
