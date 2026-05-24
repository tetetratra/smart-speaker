package router

import (
	"context"
	"sync"

	"smart-speaker/internal/graph"
	types "smart-speaker/internal/types"
)

type Config struct{}

type stage struct {
	upstream   chan types.Event
	downstream chan types.Event
	once       sync.Once
	cancel     context.CancelFunc
}

func NewStage(cfg Config) *graph.Stage {
	s := &stage{
		upstream:   make(chan types.Event, graph.DefaultChannelBufferSize),
		downstream: make(chan types.Event, graph.DefaultChannelBufferSize),
	}
	return &graph.Stage{
		Upstream:   s.upstream,
		Downstream: s.downstream,
		Run:        s.run,
		CloseFn:    s.close,
	}
}

func (s *stage) run(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	go s.consume(ctx)
}

func (s *stage) consume(ctx context.Context) {
	defer close(s.downstream)
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-s.upstream:
			if !ok {
				return
			}
			if evt.Kind != types.EventScheduledItem {
				continue
			}
			s.route(ctx, evt.Payload)
		}
	}
}

func (s *stage) route(ctx context.Context, payload any) {
	switch item := payload.(type) {
	case types.PlayableSpeech:
		s.emit(ctx, types.Event{Kind: types.EventRealtimeAudio, Payload: types.OutputAudio{
			Role:         types.RoleAgent,
			Audio:        item.Audio,
			Text:         item.Text,
			GenerationID: item.GenerationID,
		}})
		s.emit(ctx, types.Event{Kind: types.EventConversationCommitRequest, Payload: types.ConversationCommitRequest{
			Role:         types.RoleAgent,
			Text:         item.Text,
			GenerationID: item.GenerationID,
			Source:       "llm",
		}})
	case types.ToolRequest:
		s.emit(ctx, types.Event{Kind: types.EventConversationCommitRequest, Payload: types.ConversationCommitRequest{
			Role:         types.RoleToolCall,
			GenerationID: item.GenerationID,
			Source:       item.Name,
			ToolCall: &types.ToolCallRecord{
				ToolCallID:   item.ToolCallID,
				Name:         item.Name,
				Arguments:    item.Arguments,
				GenerationID: item.GenerationID,
			},
		}})
		s.emit(ctx, types.Event{Kind: types.EventToolRequest, Payload: item})
	}
}

func (s *stage) emit(ctx context.Context, evt types.Event) {
	select {
	case <-ctx.Done():
	case s.downstream <- evt:
	}
}

func (s *stage) close() error {
	s.once.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		close(s.upstream)
	})
	return nil
}
