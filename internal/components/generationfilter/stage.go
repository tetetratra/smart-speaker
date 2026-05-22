package generationfilter

import (
	"context"
	"sync"

	"smart-speaker/internal/graph"
	"smart-speaker/internal/states/generation"
	types "smart-speaker/internal/types"
)

type Config struct {
	Generation *generation.Store
}

type stage struct {
	upstream   chan types.Event
	downstream chan types.Event
	generation *generation.Store
	once       sync.Once
	cancel     context.CancelFunc
}

func NewStage(cfg Config) *graph.Stage {
	s := &stage{
		upstream:   make(chan types.Event, graph.DefaultChannelBufferSize),
		downstream: make(chan types.Event, graph.DefaultChannelBufferSize),
		generation: cfg.Generation,
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
			if !s.allow(evt) {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case s.downstream <- evt:
			}
		}
	}
}

func (s *stage) allow(evt types.Event) bool {
	if s.generation == nil {
		return true
	}
	id, ok := eventGenerationID(evt)
	if !ok {
		return false
	}
	return s.generation.IsCurrent(id)
}

func eventGenerationID(evt types.Event) (types.GenerationID, bool) {
	switch payload := evt.Payload.(type) {
	case types.TimelineItem:
		return payload.GenerationID, true
	case types.PlayableSpeech:
		return payload.GenerationID, true
	case types.ToolRequest:
		return payload.GenerationID, true
	case types.OutputAudio:
		return payload.GenerationID, true
	case types.ConversationCommitRequest:
		return payload.GenerationID, true
	default:
		return 0, false
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
