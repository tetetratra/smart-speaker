package realtime

import (
	"context"
	"testing"
)

func TestInboundMessageWaitsForConversation(t *testing.T) {
	s := &stage{speaking: true}
	s.queueInbound(testInput("input", "室温を知らせて"))
	if s.startInbound(context.Background()) || len(s.inbound) != 1 {
		t.Fatal(s.inbound)
	}
}

// 時間のかかるツールの実行中でも、予約ジョブは時刻どおりに始める。再生端末がつながっていなくても待たない。
func TestInboundMessageStartsWhileToolIsRunning(t *testing.T) {
	s := newToolResponseStage(t)
	s.calls["slow"] = &call{epoch: 1, generation: 1}
	s.queueInbound(testInput("input", "室温を知らせて"))
	if !s.startInbound(context.Background()) || len(s.inbound) != 0 {
		t.Fatal(s.inbound)
	}
}

func TestInboundWaitsForEachBusyState(t *testing.T) {
	cases := map[string]func(*stage){
		"発話中":  func(s *stage) { s.speaking = true },
		"応答中":  func(s *stage) { s.active = "response" },
		"応答待ち": func(s *stage) { s.wantResponse = true },
		"接続中":  func(s *stage) { s.connecting = true },
		"修復中":  func(s *stage) { s.repairs = map[string]bool{"item": true} },
		"出力待ち": func(s *stage) {
			s.pendingOutput = map[string]bool{"item": true}
			s.items = map[string]*assistantItem{"item": {generation: s.cfg.Generation.Current()}}
		},
	}
	for name, busy := range cases {
		t.Run(name, func(t *testing.T) {
			s := newToolResponseStage(t)
			busy(s)
			s.queueInbound(testInput("input", "結果"))
			if s.startInbound(context.Background()) || len(s.inbound) != 1 {
				t.Fatal("処理中に差し込みました")
			}
		})
	}
}

func TestInboundKeepsFIFO(t *testing.T) {
	s := newToolResponseStage(t)
	for _, text := range []string{"最初", "次"} {
		s.queueInbound(testInput(text, text))
	}
	if !s.startInbound(context.Background()) || s.startInbound(context.Background()) || len(s.inbound) != 1 {
		t.Fatal(s.inbound)
	}
	s.active, s.wantResponse = "", false
	if !s.startInbound(context.Background()) || len(s.inbound) != 0 {
		t.Fatal(s.inbound)
	}
}
