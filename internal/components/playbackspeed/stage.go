package playbackspeed

import (
	"context"
	"sync"

	"github.com/tetetratra/smart-speaker/internal/graph"
	pbspeed "github.com/tetetratra/smart-speaker/internal/states/playbackspeed"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

type Config struct {
	Store *pbspeed.Store
}

type stage struct {
	upstream   chan types.Event
	downstream chan types.Event
	store      *pbspeed.Store
	once       sync.Once
	cancel     context.CancelFunc
}

func NewStage(cfg Config) *graph.Stage {
	s := &stage{
		upstream:   make(chan types.Event, graph.DefaultChannelBufferSize),
		downstream: make(chan types.Event, graph.DefaultChannelBufferSize),
		store:      cfg.Store,
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
			processed, err := s.transform(evt)
			if err != nil {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case s.downstream <- processed:
			}
		}
	}
}

func (s *stage) speed() float64 {
	if s.store == nil {
		return 1
	}
	return s.store.Speed()
}

func (s *stage) transform(evt types.Event) (types.Event, error) {
	speed := s.speed()
	switch payload := evt.Payload.(type) {
	case types.PlayableSpeech:
		updated, err := applyToSpeech(payload, speed)
		if err != nil {
			return types.Event{}, err
		}
		return types.Event{Kind: evt.Kind, Payload: updated}, nil
	case types.TimelineItem:
		if payload.Kind != types.TimelineKindWait {
			return evt, nil
		}
		return types.Event{Kind: evt.Kind, Payload: applyToWait(payload, speed)}, nil
	default:
		return evt, nil
	}
}

func applyToSpeech(speech types.PlayableSpeech, speed float64) (types.PlayableSpeech, error) {
	if speed <= 0 || speed == 1 {
		return speech, nil
	}
	audio, err := compressPCM(speech.Audio, speed)
	if err != nil {
		return types.PlayableSpeech{}, err
	}
	speech.Audio = audio
	speech.DurationSeconds /= speed
	return speech, nil
}

func applyToWait(item types.TimelineItem, speed float64) types.TimelineItem {
	if speed <= 0 || speed == 1 {
		return item
	}
	item.Sec /= speed
	return item
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
