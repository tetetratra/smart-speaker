package realtime

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/tetetratra/smart-speaker/internal/components/generationcontroller"
	"github.com/tetetratra/smart-speaker/internal/eventqueue"
	"github.com/tetetratra/smart-speaker/internal/graph"
	"github.com/tetetratra/smart-speaker/internal/states/generation"
	"github.com/tetetratra/smart-speaker/internal/types"
)

// newTestStage は、接続済みの状態の stage を、イベントループを動かさずに作る。送信は transport.commands にたまる。
func newTestStage(t *testing.T) *stage {
	gen := generation.NewStore()
	gen.Next()
	return &stage{
		cfg:           Config{Generation: gen, History: newTestHistory(t)},
		transcription: &graph.Stage{Downstream: make(chan types.Event, 100)},
		response:      &graph.Stage{Downstream: make(chan types.Event, 100)},
		transport:     &transport{commands: eventqueue.New[any](nil)},
		controller:    generationcontroller.New(gen), epoch: 1, ready: true,
		started:   time.Now(),
		responses: map[string]*response{}, userGenerations: map[string]types.GenerationID{},
		items: map[string]*assistantItem{}, pendingOutput: map[string]bool{}, repairs: map[string]bool{}, commentary: map[string]bool{},
	}
}

// popCommand は、応答の前に毎回足す知らせを読み飛ばし、次の API 要求が kind であることを確かめて返す。
func popCommand(t *testing.T, s *stage, kind string) map[string]any {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cmd, ok := s.transport.commands.Pop(ctx)
	if ok && isNotice(cmd.(map[string]any)) {
		cmd, ok = s.transport.commands.Pop(ctx)
	}
	if !ok || cmd.(map[string]any)["type"] != kind {
		t.Fatalf("期待するAPI要求がありません: %s, %v", kind, cmd)
	}
	return cmd.(map[string]any)
}

// popResponse は、応答の前の知らせと応答要求を順に取り出す。
func popResponse(t *testing.T, s *stage) (map[string]any, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cmd, ok := s.transport.commands.Pop(ctx)
	if !ok || !isNotice(cmd.(map[string]any)) {
		t.Fatalf("応答の前の知らせがありません: %v", cmd)
	}
	return popCommand(t, s, "response.create")["response"].(map[string]any), noticeText(cmd.(map[string]any))
}

func noCommand(t *testing.T, s *stage) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if cmd, ok := s.transport.commands.Pop(ctx); ok {
		t.Fatalf("不要なAPI要求です: %v", cmd)
	}
}

func isNotice(cmd map[string]any) bool {
	item, _ := cmd["item"].(map[string]any)
	return cmd["type"] == "conversation.item.create" && item != nil && item["role"] == "system"
}

func noticeText(cmd map[string]any) string {
	return cmd["item"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
}

func serverEventForTest(s *stage, event any) {
	raw, _ := json.Marshal(event)
	s.serverEvent(context.Background(), raw)
}
