// Package conversationmanager は会話の記録と外部入力を集約する。外部入力は gateway からイベントとして届く。
package conversationmanager

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
	s.node = &graph.Stage{Name: "conversationmanager", Upstream: make(chan types.Event), Downstream: make(chan types.Event), Run: s.run, CloseFn: func() error { return nil }}
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
		if input.ModelHandling == types.ModelHandlingAlreadyKnown {
			s.save(ctx, input.Record)
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
		s.save(ctx, input.Record)
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
// ホワイトボードに出す記録（会話でないもの）は、保存したあとで画面へ知らせる。
func (s *stage) save(ctx context.Context, record types.ConversationRecord) {
	if err := s.history.Upsert(record); err != nil {
		log.Printf("会話管理: 会話履歴の保存: %v", err)
	}
	if !record.IsDialogue() {
		s.emit(ctx, types.EventConversationRecordChanged{ConversationRecord: record})
	}
}
