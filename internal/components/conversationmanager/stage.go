// Package conversationmanager は会話の記録と外部入力を集約する。
package conversationmanager

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/tetetratra/smart-speaker/internal/automations/script"
	"github.com/tetetratra/smart-speaker/internal/eventqueue"
	"github.com/tetetratra/smart-speaker/internal/graph"
	"github.com/tetetratra/smart-speaker/internal/states/conversationhistory"
	"github.com/tetetratra/smart-speaker/internal/states/toolhistory"
	"github.com/tetetratra/smart-speaker/internal/types"
)

type Config struct {
	Inbound     <-chan types.InboundMessage
	ToolHistory *toolhistory.Store
	History     *conversationhistory.Store
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
	// グラフと外部通知を一つのキューへ集め、保存と引き渡しの順序を揃える。
	queue := eventqueue.New[types.Event](nil)
	go func() {
		for {
			e, ok := s.node.Receive(ctx)
			if !ok {
				return
			}
			queue.Push(e)
		}
	}()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case message, ok := <-s.cfg.Inbound:
				if !ok {
					return
				}
				queue.Push(InputEvent(message))
			}
		}
	}()
	for {
		e, ok := queue.Pop(ctx)
		if !ok {
			return
		}
		s.handle(ctx, e)
	}
}

// InputEvent は既存の非同期通知を会話への情報追加・応答依頼へ変換する。
func InputEvent(message types.InboundMessage) types.Event {
	handling := types.ModelHandlingAddAndRespond
	if types.HistoryOnly(message.Context) {
		handling = types.ModelHandlingAddToContext
	}
	// 通知方針は制御情報なので履歴には残さない。依頼の識別情報は維持する。
	var metadata map[string]json.RawMessage
	_ = json.Unmarshal(message.Context, &metadata)
	delete(metadata, "delivery")
	record := types.ConversationRecord{ID: message.ID, Role: message.Role, Text: message.Text, Source: message.Source}
	if len(metadata) > 0 {
		raw, _ := json.Marshal(metadata)
		record.Metadata = map[string]any{"context": string(raw)}
	}
	if record.Role == "" {
		record.Role = types.RoleSystem
	}
	return types.Event{Kind: types.EventConversationInput, Payload: types.ConversationInput{Record: record, ModelHandling: handling, Title: message.Title, External: true}}
}

func (s *stage) handle(ctx context.Context, e types.Event) {
	switch e.Kind {
	case types.EventConversationInput:
		input := e.Payload.(types.ConversationInput)
		if input.ModelHandling == types.ModelHandlingAlreadyKnown {
			s.save(input.Record)
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
		s.save(input.Record)
		s.unused[input.Record.ID] = true
		if input.External && s.cfg.ToolHistory != nil && (input.Record.Source != script.Source || input.Record.Role == types.RoleSystem) {
			s.cfg.ToolHistory.RecordInbound(types.InboundMessage{ID: input.Record.ID, Source: input.Record.Source, Title: input.Title, Text: input.Record.Text}, input.Record.GenerationID)
		}
		e.Payload = input
		s.emit(ctx, e)
	case types.EventConversationHistorySync:
		s.emit(ctx, e)
		select {
		case <-e.Payload.(types.ConversationHistorySync).Done:
		case <-ctx.Done():
		}
	case types.EventConversationRecordsUsed:
		for _, id := range e.Payload.(types.ConversationRecordsUsed).IDs {
			delete(s.unused, id)
		}
	case types.EventConversationIdle:
		if s.cfg.OnIdle != nil {
			s.cfg.OnIdle(ctx, s.unused)
		}
	}
}

// 保存に失敗しても会話は続けられるので、記録はメモリ上に残してログだけ出す。
func (s *stage) save(record types.ConversationRecord) {
	if err := s.history.Upsert(record); err != nil {
		log.Printf("会話管理: 会話履歴の保存: %v", err)
	}
}
