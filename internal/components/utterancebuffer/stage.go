package utterancebuffer

import (
	"context"
	"log"
	"sync"
	"time"

	"smart-speaker/internal/graph"
	"smart-speaker/internal/states/generation"
	types "smart-speaker/internal/types"
)

type Config struct {
	Delay      time.Duration
	Generation *generation.Store
}

type stage struct {
	upstream   chan types.Event
	downstream chan types.Event
	generation *generation.Store
	delay      time.Duration
	once       sync.Once
	cancel     context.CancelFunc
}

func NewStage(cfg Config) *graph.Stage {
	if cfg.Delay <= 0 {
		cfg.Delay = 500 * time.Millisecond
	}
	s := &stage{
		upstream:   make(chan types.Event, graph.DefaultChannelBufferSize),
		downstream: make(chan types.Event, graph.DefaultChannelBufferSize),
		generation: cfg.Generation,
		delay:      cfg.Delay,
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
	var buf buffer
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
		if timer == nil {
			timer = time.NewTimer(s.delay)
			timerC = timer.C
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(s.delay)
		timerC = timer.C
	}
	flush := func() {
		if buf.empty() {
			return
		}
		if s.generation == nil {
			log.Printf("utterancebuffer: generation store is nil")
			buf.reset()
			return
		}
		text := buf.text()
		buf.reset()
		generationID := s.generation.Next()
		s.emit(ctx, types.Event{
			Kind: types.EventConversationCommitRequest,
			Payload: types.ConversationCommitRequest{
				Role:         types.RoleUser,
				Text:         text,
				GenerationID: generationID,
				Source:       "stt",
			},
		})
	}
	defer stopTimer()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timerC:
			timer = nil
			timerC = nil
			flush()
		case evt, ok := <-s.upstream:
			if !ok {
				flush()
				return
			}
			if evt.Kind != types.EventHumanUtterance {
				continue
			}
			line, ok := evt.Payload.(types.OutputLine)
			if !ok || line.Text == "" {
				continue
			}
			buf.append(line.Text)
			resetTimer()
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
