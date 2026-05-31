package scheduler

import (
	"context"
	"sync"
	"time"

	"github.com/tetetratra/smart-speaker/internal/graph"
	pbspeed "github.com/tetetratra/smart-speaker/internal/states/playbackspeed"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

type Config struct {
	SpeedStore *pbspeed.Store
}

type stage struct {
	upstream   chan types.Event
	downstream chan types.Event
	once       sync.Once
	cancel     context.CancelFunc
	speedStore *pbspeed.Store

	mu      sync.Mutex
	workers map[types.GenerationID]chan types.Event
}

func NewStage(cfg Config) *graph.Stage {
	s := &stage{
		upstream:   make(chan types.Event, graph.DefaultChannelBufferSize),
		downstream: make(chan types.Event, graph.DefaultChannelBufferSize),
		speedStore: cfg.SpeedStore,
		workers:    map[types.GenerationID]chan types.Event{},
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
			s.closeWorkers()
			return
		case evt, ok := <-s.upstream:
			if !ok {
				s.closeWorkers()
				return
			}
			id, ok := generationID(evt)
			if !ok {
				continue
			}
			s.enqueue(ctx, id, evt)
		}
	}
}

func (s *stage) enqueue(ctx context.Context, id types.GenerationID, evt types.Event) {
	s.mu.Lock()
	ch := s.workers[id]
	if ch == nil {
		ch = make(chan types.Event, graph.DefaultChannelBufferSize)
		s.workers[id] = ch
		go s.runGeneration(ctx, id, ch)
	}
	s.mu.Unlock()
	select {
	case <-ctx.Done():
	case ch <- evt:
	}
}

func (s *stage) runGeneration(ctx context.Context, id types.GenerationID, ch <-chan types.Event) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-ch:
			if !ok {
				return
			}
			s.handle(ctx, evt)
		}
		_ = id
	}
}

func (s *stage) handle(ctx context.Context, evt types.Event) {
	switch payload := evt.Payload.(type) {
	case types.AgentTimelineEnd:
		s.emit(ctx, types.Event{
			Kind: types.EventAgentSpeechPlaybackEnd,
			Payload: types.AgentSpeechPlaybackEnd{
				GenerationID: payload.GenerationID,
				CompletedAt:  time.Now(),
			},
		})
	case types.PlayableSpeech:
		s.emit(ctx, types.Event{Kind: types.EventScheduledItem, Payload: payload})
		s.wait(ctx, payload.DurationSeconds)
	case types.TimelineItem:
		switch payload.Kind {
		case types.TimelineKindWait:
			s.wait(ctx, s.adjustWaitSeconds(payload.Sec))
		case types.TimelineKindTool:
			s.emit(ctx, types.Event{Kind: types.EventScheduledItem, Payload: types.ToolRequest{
				ToolCallID:   payload.SequenceID,
				Name:         payload.ToolName,
				Arguments:    payload.ToolArgs,
				GenerationID: payload.GenerationID,
				SequenceID:   payload.SequenceID,
			}})
		}
	}
}

func (s *stage) adjustWaitSeconds(seconds float64) float64 {
	speed := s.playbackSpeed()
	if speed <= 0 || speed == 1 {
		return seconds
	}
	return seconds / speed
}

func (s *stage) playbackSpeed() float64 {
	if s.speedStore == nil {
		return 1
	}
	return s.speedStore.Speed()
}

func (s *stage) wait(ctx context.Context, seconds float64) {
	if seconds <= 0 {
		return
	}
	timer := time.NewTimer(time.Duration(seconds * float64(time.Second)))
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

func (s *stage) emit(ctx context.Context, evt types.Event) {
	select {
	case <-ctx.Done():
	case s.downstream <- evt:
	}
}

func generationID(evt types.Event) (types.GenerationID, bool) {
	switch payload := evt.Payload.(type) {
	case types.PlayableSpeech:
		return payload.GenerationID, true
	case types.TimelineItem:
		return payload.GenerationID, true
	case types.AgentTimelineEnd:
		return payload.GenerationID, true
	default:
		return 0, false
	}
}

func (s *stage) closeWorkers() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, ch := range s.workers {
		close(ch)
		delete(s.workers, id)
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
