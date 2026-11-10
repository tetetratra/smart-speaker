package realtime

import (
	"context"
	"testing"

	"github.com/tetetratra/smart-speaker/internal/types"
)

func TestScheduledPromptWaitsForConversation(t *testing.T) {
	s := &stage{speaking: true}
	s.queueScheduled(types.ScheduledPrompt{ID: "job", Instruction: "室温を知らせて"})
	if s.startScheduled(context.Background()) || len(s.scheduled) != 1 {
		t.Fatal(s.scheduled)
	}
}

// 時間のかかるツールの実行中でも、予約ジョブは時刻どおりに始める。再生端末がつながっていなくても待たない。
func TestScheduledPromptStartsWhileToolIsRunning(t *testing.T) {
	s := newToolResponseStage()
	s.calls["slow"] = &call{epoch: 1, generation: 1}
	s.queueScheduled(types.ScheduledPrompt{ID: "job", Instruction: "室温を知らせて"})
	if !s.startScheduled(context.Background()) || len(s.scheduled) != 0 {
		t.Fatal(s.scheduled)
	}
}
