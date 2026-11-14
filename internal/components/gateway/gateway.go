// Package gateway は、グラフの外とグラフの境目を受け持つ。
// 予約ジョブ実行・Claude Code 連携・Web 検索の結果など、グラフの外で動く処理からの入力は、すべてここを通ってグラフへ入る。
// 逆に、main で gateway へ配線したグラフのイベントは、購読しているグラフの外の処理へ渡す。
package gateway

import (
	"context"

	"github.com/tetetratra/smart-speaker/internal/eventqueue"
	"github.com/tetetratra/smart-speaker/internal/graph"
	"github.com/tetetratra/smart-speaker/internal/types"
)

// Deliver は、グラフの外から会話へ入力を1件届ける。
type Deliver func(context.Context, types.ConversationInput) error

// Gateway はグラフの外からの入力を受け取り、イベントとしてグラフへ流す。
// サービスはグラフのコンポーネントより先に作られるので、Gateway を先に作って Deliver を渡す。
type Gateway struct {
	node        *graph.Stage
	queue       *eventqueue.Queue[types.Event]
	subscribers []func(context.Context, types.Event)
}

// Subscribe は、gateway へ配線したイベントを受け取る処理を足す。グラフを動かす前に呼ぶ。
// イベントは届いた順に1つずつ渡すので、処理が終わるまで次のイベントは渡さない。
func (g *Gateway) Subscribe(fn func(context.Context, types.Event)) {
	g.subscribers = append(g.subscribers, fn)
}

func New() *Gateway {
	g := &Gateway{queue: eventqueue.New[types.Event](nil)}
	g.node = &graph.Stage{Name: "gateway", Upstream: make(chan types.Event), Downstream: make(chan types.Event), Run: g.run, CloseFn: func() error { return nil }}
	return g
}

// Stage はグラフに載せるコンポーネントを返す。
func (g *Gateway) Stage() *graph.Stage { return g.node }

// Deliver は入力をキューに積むだけで、グラフの空きを待たない。
func (g *Gateway) Deliver(ctx context.Context, input types.ConversationInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	g.queue.Push(types.EventConversationInput{ConversationInput: input})
	return nil
}

func (g *Gateway) run(ctx context.Context) {
	defer close(g.node.Downstream)
	go func() {
		for {
			e, ok := g.node.Receive(ctx)
			if !ok {
				return
			}
			for _, fn := range g.subscribers {
				fn(ctx, e)
			}
		}
	}()
	for {
		e, ok := g.queue.Pop(ctx)
		if !ok {
			return
		}
		select {
		case g.node.Downstream <- e:
		case <-ctx.Done():
			return
		}
	}
}
