package sessionreset

import (
	"context"
	"sync"
	"time"

	"github.com/tetetratra/smart-speaker/internal/graph"
	"github.com/tetetratra/smart-speaker/internal/states/conversationhistory"
	"github.com/tetetratra/smart-speaker/internal/states/generation"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

type Config struct {
	IdleTimeout time.Duration
	History     *conversationhistory.Store
	Generation  *generation.Store
	Hooks       []Hook
	Now         func() time.Time
}

type Hook interface {
	Exec(context.Context) error
}

type stage struct {
	upstream    chan types.Event
	downstream  chan types.Event
	idleTimeout time.Duration
	history     *conversationhistory.Store
	generation  *generation.Store
	hooks       []Hook
	now         func() time.Time
	once        sync.Once
	cancel      context.CancelFunc
}

func NewStage(cfg Config) *graph.Stage {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	hooks := append([]Hook(nil), cfg.Hooks...)
	s := &stage{
		upstream:    make(chan types.Event, graph.DefaultChannelBufferSize),
		downstream:  make(chan types.Event, graph.DefaultChannelBufferSize),
		idleTimeout: cfg.IdleTimeout,
		history:     cfg.History,
		generation:  cfg.Generation,
		hooks:       hooks,
		now:         now,
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
		case _, ok := <-s.upstream:
			if !ok {
				return
			}
		}
	}
}

func (s *stage) fireReset(ctx context.Context) {
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
