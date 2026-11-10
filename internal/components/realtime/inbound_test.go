package realtime

import (
	"context"
	"testing"

	"github.com/tetetratra/smart-speaker/internal/types"
)

func TestInboundMessageWaitsForConversation(t *testing.T) {
	s := &stage{speaking: true}
	s.queueInbound(types.InboundMessage{Source: "automation/job", Text: "室温を知らせて"})
	if s.startInbound(context.Background()) || len(s.inbound) != 1 {
		t.Fatal(s.inbound)
	}
}

// 時間のかかるツールの実行中でも、予約ジョブは時刻どおりに始める。再生端末がつながっていなくても待たない。
func TestInboundMessageStartsWhileToolIsRunning(t *testing.T) {
	s := newToolResponseStage()
	s.calls["slow"] = &call{epoch: 1, generation: 1}
	s.queueInbound(types.InboundMessage{Source: "automation/job", Text: "室温を知らせて"})
	if !s.startInbound(context.Background()) || len(s.inbound) != 0 {
		t.Fatal(s.inbound)
	}
}
