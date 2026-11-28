// Package conversationrecorder は会話の記録を保存して配る。記録は realtime・頭脳AI・gateway・予約ジョブから、イベントとして届く。
// 保存した記録（EventConversationInput）は、すべて会話の流れ（conversationflow）と予約ジョブのコンポーネントへ流す。
// どう応答させるかは会話の流れが決め、予約ジョブ側は購読可能な記録だけを扱う。
package conversationrecorder

import (
	"context"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/tetetratra/smart-speaker/internal/graph"
	"github.com/tetetratra/smart-speaker/internal/states/conversationhistory"
	"github.com/tetetratra/smart-speaker/internal/types"
)

type Config struct {
	History *conversationhistory.Store
	// OnIdle は無操作が続いたときに呼ぶ。まだ応答に使っていない外部記録のIDを渡す。
	OnIdle func(context.Context, map[string]bool)
}

type stage struct {
	node    *graph.Stage
	cfg     Config
	history *conversationhistory.Store
	seen    map[string]bool
	unused  map[string]bool
}

func NewStage(cfg Config) *graph.Stage {
	s := &stage{cfg: cfg, history: cfg.History, seen: map[string]bool{}, unused: map[string]bool{}}
	s.node = &graph.Stage{Name: "conversationrecorder", Upstream: make(chan types.Event), Downstream: make(chan types.Event), Run: s.run, CloseFn: func() error { return nil }}
	return s.node
}

func (s *stage) emit(ctx context.Context, e types.Event) {
	select {
	case s.node.Downstream <- e:
	case <-ctx.Done():
	}
}

func (s *stage) run(ctx context.Context) {
	defer close(s.node.Downstream)
	// 外からの入力も gateway からイベントとして届くので、グラフのイベントを届いた順に処理するだけでよい。
	for {
		e, ok := s.node.Receive(ctx)
		if !ok {
			return
		}
		s.handle(ctx, e)
	}
}

func (s *stage) handle(ctx context.Context, e types.Event) {
	switch e := e.(type) {
	case types.EventConversationInput:
		input := e.ConversationInput
		if input.ModelAlreadyKnows {
			// 会話の流れがユーザーの発話や頭脳AIの話す内容を見て次を決めるので、保存したあとで流す。
			s.save(ctx, input)
			s.emit(ctx, types.EventConversationInput{ConversationInput: input})
			return
		}
		if input.Record.ID == "" {
			input.Record.ID = "input/" + uuid.NewString()
		}
		if s.seen[input.Record.ID] {
			return
		}
		s.seen[input.Record.ID] = true
		if input.Record.CreatedAt.IsZero() {
			input.Record.CreatedAt = time.Now()
		}
		s.save(ctx, input)
		s.unused[input.Record.ID] = true
		s.emit(ctx, types.EventConversationInput{ConversationInput: input})
	case types.EventConversationHistorySync:
		s.emit(ctx, e)
		select {
		case <-e.Done:
		case <-ctx.Done():
		}
	case types.EventConversationRecordsUsed:
		for _, id := range e.IDs {
			delete(s.unused, id)
		}
	case types.EventConversationIdle:
		if s.cfg.OnIdle != nil {
			s.cfg.OnIdle(ctx, s.unused)
		}
	}
}

// 保存に失敗しても会話は続けられるので、記録はメモリ上に残してログだけ出す。
// UserAlreadyKnows が立っていない記録は、保存したあとで画面へ送る。どこに出すか（チャット欄・ホワイトボード）は画面が決める。
func (s *stage) save(ctx context.Context, input types.ConversationInput) {
	if err := s.history.Append(input.Record); err != nil {
		log.Printf("会話の記録: 会話履歴の保存: %v", err)
	}
	if !input.UserAlreadyKnows {
		s.emit(ctx, types.EventConversationRecordAdded{ConversationRecord: input.Record})
	}
}
