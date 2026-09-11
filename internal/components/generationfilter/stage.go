package generationfilter

import (
	"context"
	"log"
	"sync"

	"github.com/tetetratra/smart-speaker/internal/graph"
	"github.com/tetetratra/smart-speaker/internal/states/generation"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

const defaultMaxHeldEvents = 256

type Config struct {
	Generation    *generation.Store
	MaxHeldEvents int
}

type stage struct {
	upstream      chan types.Event
	downstream    chan types.Event
	generation    *generation.Store
	maxHeldEvents int
	once          sync.Once
	cancel        context.CancelFunc
}

func NewStage(cfg Config) *graph.Stage {
	if cfg.MaxHeldEvents <= 0 {
		cfg.MaxHeldEvents = defaultMaxHeldEvents
	}
	s := &stage{
		upstream:      make(chan types.Event, graph.DefaultChannelBufferSize),
		downstream:    make(chan types.Event, graph.DefaultChannelBufferSize),
		generation:    cfg.Generation,
		maxHeldEvents: cfg.MaxHeldEvents,
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
	var updates <-chan struct{}
	var unsubscribe func()
	if s.generation != nil {
		updates, unsubscribe = s.generation.Subscribe()
	}
	if unsubscribe != nil {
		defer unsubscribe()
	}
	held := make([]types.Event, 0)
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-updates:
			if !ok {
				updates = nil
				continue
			}
			var done bool
			held, done = s.flushHeld(ctx, held)
			if done {
				return
			}
		case evt, ok := <-s.upstream:
			if !ok {
				return
			}
			decision := s.disposition(evt)
			switch decision {
			case generation.EventDispositionAllow:
				if s.emit(ctx, evt) {
					return
				}
			case generation.EventDispositionHold:
				held = s.appendHeld(held, evt)
			case generation.EventDispositionDrop:
				continue
			}
		}
	}
}

func (s *stage) disposition(evt types.Event) generation.EventDisposition {
	if s.generation == nil {
		return generation.EventDispositionAllow
	}
	id, ok := eventGenerationID(evt)
	if !ok {
		return generation.EventDispositionDrop
	}
	return s.generation.Disposition(id)
}

func (s *stage) appendHeld(held []types.Event, evt types.Event) []types.Event {
	if len(held) >= s.maxHeldEvents {
		log.Printf("generationfilter: drop oldest held event max_held_events=%d", s.maxHeldEvents)
		copy(held, held[1:])
		held[len(held)-1] = evt
		return held
	}
	return append(held, evt)
}

func (s *stage) flushHeld(ctx context.Context, held []types.Event) ([]types.Event, bool) {
	if len(held) == 0 {
		return held, false
	}
	next := held[:0]
	for _, evt := range held {
		switch s.disposition(evt) {
		case generation.EventDispositionAllow:
			if s.emit(ctx, evt) {
				return next, true
			}
		case generation.EventDispositionHold:
			next = append(next, evt)
		case generation.EventDispositionDrop:
			continue
		}
	}
	return next, false
}

func (s *stage) emit(ctx context.Context, evt types.Event) bool {
	select {
	case <-ctx.Done():
		return true
	case s.downstream <- evt:
		return false
	}
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
	case types.AgentTimelineEnd:
		return payload.GenerationID, true
	case types.AgentSpeechPlaybackEnd:
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
