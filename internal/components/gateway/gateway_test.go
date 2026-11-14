package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/tetetratra/smart-speaker/internal/types"
)

// 送り口はグラフの空きを待たず、届いた順にイベントとしてグラフへ流す。
func TestDeliverFlowsAsEventsInOrder(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	g := New()
	for _, id := range []string{"first", "second"} {
		if err := g.Deliver(ctx, types.ConversationInput{Record: types.ConversationRecord{ID: id}, ModelHandling: types.ModelHandlingAddToContext}); err != nil {
			t.Fatal(err)
		}
	}
	go g.Stage().Run(ctx)
	for _, want := range []string{"first", "second"} {
		select {
		case e := <-g.Stage().Downstream:
			input, ok := e.(types.EventConversationInput)
			if !ok || input.Record.ID != want || input.ModelHandling != types.ModelHandlingAddToContext {
				t.Fatal(e)
			}
		case <-ctx.Done():
			t.Fatal("イベントが流れません")
		}
	}
}

func TestDeliverRejectsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := New().Deliver(ctx, types.ConversationInput{}); err == nil {
		t.Fatal("取り消された context でも受け付けました")
	}
}

// gateway へ配線されたイベントは、購読している処理へ届いた順に1つずつ渡す。
func TestSubscribersReceiveEventsInOrder(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	g := New()
	got := make(chan types.Event, 2)
	g.Subscribe(func(_ context.Context, e types.Event) { got <- e })
	go g.Stage().Run(ctx)
	g.Stage().Upstream <- types.EventConversationIdle{}
	g.Stage().Upstream <- types.EventConversationRecordsUsed{IDs: []string{"a"}}
	for _, want := range []string{"EventConversationIdle", "EventConversationRecordsUsed"} {
		select {
		case e := <-got:
			if types.EventName(e) != want {
				t.Fatal(e)
			}
		case <-ctx.Done():
			t.Fatal("イベントが届きません")
		}
	}
}
