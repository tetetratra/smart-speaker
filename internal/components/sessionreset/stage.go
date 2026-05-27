package sessionreset

import (
	"context"
	"log"
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
	var timer *time.Timer
	var timerC <-chan time.Time
	stopTimer := func() {
		if timer == nil {
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer = nil
		timerC = nil
	}
	resetTimer := func() {
		if s.idleTimeout <= 0 {
			return
		}
		if timer == nil {
			timer = time.NewTimer(s.idleTimeout)
			timerC = timer.C
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(s.idleTimeout)
		timerC = timer.C
	}
	defer stopTimer()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timerC:
			timer = nil
			timerC = nil
			reset := s.fireReset(ctx)
			select {
			case s.downstream <- types.Event{Kind: types.EventSessionReset, Payload: reset}:
			case <-ctx.Done():
				return
			}
		case evt, ok := <-s.upstream:
			if !ok {
				return
			}
			if s.isUserCommitRequest(evt) {
				resetTimer()
			}
		}
	}
}

func (s *stage) isUserCommitRequest(evt types.Event) bool {
	if evt.Kind != types.EventConversationCommitRequest {
		return false
	}
	req, ok := evt.Payload.(types.ConversationCommitRequest)
	return ok && req.Role == types.RoleUser
}

func (s *stage) fireReset(ctx context.Context) types.SessionResetEvent {
	requestedAt := s.now()
	formatted := requestedAt.Format(time.RFC3339Nano)
	log.Printf("sessionreset: reset requested_at=%s", formatted)
	for _, hook := range s.hooks {
		if hook == nil {
			continue
		}
		if err := hook.Exec(ctx); err != nil {
			log.Printf("sessionreset: hook error requested_at=%s err=%v", formatted, err)
		}
	}
	if s.history != nil {
		s.history.Reset()
	}
	if s.generation != nil {
		next := s.generation.Next()
		log.Printf("sessionreset: generation advanced requested_at=%s generation=%d", formatted, next)
	}
	return types.SessionResetEvent{RequestedAt: requestedAt}
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
