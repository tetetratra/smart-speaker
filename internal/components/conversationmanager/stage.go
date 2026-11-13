// Package conversationmanager は会話の記録と外部入力を集約する。
package conversationmanager

import (
	"context"
	"encoding/json"
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
	Inbound           <-chan types.InboundMessage
	ToolHistory       *toolhistory.Store
	History           *conversationhistory.Store
	OnConversationEnd func(context.Context, map[string]bool)
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
	kind := types.EventConversationResponseRequest
	if types.HistoryOnly(message.Context) {
		kind = types.EventConversationContextAddition
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
	return types.Event{Kind: kind, Payload: types.ConversationInput{Record: record, Title: message.Title, External: true}}
}

func (s *stage) handle(ctx context.Context, e types.Event) {
	switch e.Kind {
	case types.EventConversationRecord:
		record := e.Payload.(types.ConversationRecord)
		s.history.Upsert(record)
	case types.EventConversationContextAddition, types.EventConversationResponseRequest:
		input := e.Payload.(types.ConversationInput)
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
		s.history.Upsert(input.Record)
		s.unused[input.Record.ID] = true
		if input.External && s.cfg.ToolHistory != nil && (input.Record.Source != script.Source || input.Record.Role == types.RoleSystem) {
			s.cfg.ToolHistory.RecordInbound(types.InboundMessage{ID: input.Record.ID, Source: input.Record.Source, Title: input.Title, Text: input.Record.Text}, input.Record.GenerationID)
		}
		e.Payload = input
		s.emit(ctx, e)
	case types.EventConversationHistorySync:
		close(e.Payload.(types.ConversationHistorySync).Done)
	case types.EventConversationRecordsUsed:
		for _, id := range e.Payload.(types.ConversationRecordsUsed).IDs {
			delete(s.unused, id)
		}
	case types.EventConversationEnded:
		if s.cfg.OnConversationEnd != nil {
			s.cfg.OnConversationEnd(ctx, s.unused)
		}
	}
}
