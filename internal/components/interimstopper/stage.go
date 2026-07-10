package interimstopper

import (
	"context"
	"log"
	"sync"

	"github.com/tetetratra/smart-speaker/internal/graph"
	"github.com/tetetratra/smart-speaker/internal/states/generation"
	types "github.com/tetetratra/smart-speaker/internal/types"
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
	stopped := false
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-s.upstream:
			if !ok {
				return
			}
			switch evt.Kind {
			case types.EventHumanInterimUtterance:
				if stopped {
					continue
				}
				line, ok := evt.Payload.(types.OutputLine)
				if !ok || line.Text == "" {
					continue
				}
				if s.generation == nil {
					log.Printf("interimstopper: generation store is nil")
					continue
				}
				id := s.generation.Next()
				stopped = true
				log.Printf("interimstopper: stopped current AI output at generation %d", id)
			case types.EventHumanUtterance:
				stopped = false
				s.emit(ctx, evt)
			}
		}
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
