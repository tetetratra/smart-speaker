package conversationcommitter

import (
	"context"
	"sync"

	"github.com/tetetratra/smart-speaker/internal/graph"
	"github.com/tetetratra/smart-speaker/internal/states/conversationhistory"
	"github.com/tetetratra/smart-speaker/internal/states/generation"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

type Config struct {
	History    *conversationhistory.Store
	Generation *generation.Store
}

type stage struct {
	upstream   chan types.Event
	downstream chan types.Event
	committer  *committer
	once       sync.Once
	cancel     context.CancelFunc
}

func NewStage(cfg Config) (*graph.Stage, *ResultAPI) {
	s := &stage{
		upstream:   make(chan types.Event, graph.DefaultChannelBufferSize),
		downstream: make(chan types.Event, graph.DefaultChannelBufferSize),
	}
	s.committer = &committer{
		history:    cfg.History,
		generation: cfg.Generation,
		emit:       s.emit,
	}
	api := &ResultAPI{input: s.upstream, generation: cfg.Generation}
	return &graph.Stage{
		Upstream:   s.upstream,
		Downstream: s.downstream,
		Run:        s.run,
		CloseFn:    s.close,
	}, api
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
			if evt.Kind != types.EventConversationCommitRequest {
				continue
			}
			req, ok := evt.Payload.(types.ConversationCommitRequest)
			if !ok {
				continue
			}
			s.committer.Commit(ctx, req)
		}
	}
}

func (s *stage) emit(evt types.Event) {
	select {
	case s.downstream <- evt:
	default:
		s.downstream <- evt
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
