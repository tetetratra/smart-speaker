package sessionactivate

import (
	"context"
	"sync"

	"github.com/tetetratra/smart-speaker/internal/graph"
	"github.com/tetetratra/smart-speaker/internal/states/agentstatus"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

type Config struct {
	AgentStatus *agentstatus.Store
}

type stage struct {
	upstream    chan types.Event
	downstream  chan types.Event
	agentStatus *agentstatus.Store
	once        sync.Once
	cancel      context.CancelFunc
}

func NewStage(cfg Config) *graph.Stage {
	s := &stage{
		upstream:    make(chan types.Event, graph.DefaultChannelBufferSize),
		downstream:  make(chan types.Event, graph.DefaultChannelBufferSize),
		agentStatus: cfg.AgentStatus,
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
			s.markActiveIfSpeech(evt)
			select {
			case <-ctx.Done():
				return
			case s.downstream <- evt:
			}
		}
	}
}

func (s *stage) markActiveIfSpeech(evt types.Event) {
	if s.agentStatus == nil || evt.Kind != types.EventTimelineItem {
		return
	}
	item, ok := evt.Payload.(types.TimelineItem)
	if !ok || item.Kind != types.TimelineKindSpeech {
		return
	}
	s.agentStatus.SetActive()
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
