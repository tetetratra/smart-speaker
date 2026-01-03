package turnmanager

import (
	"context"
	"strings"
	"sync"

	"smart-speaker/internal/graph"
	types "smart-speaker/internal/types"
)

type turnManager struct {
	upstream   chan types.Event
	downstream chan types.Event

	ctx    context.Context
	cancel context.CancelFunc
	once   sync.Once

	pendingFinal string
	stopSeen     bool
}

// NewStage は発話終端判定用のステージを構築します。
func NewStage() *graph.Stage {
	s := &turnManager{
		upstream:   make(chan types.Event, graph.DefaultChannelBufferSize),
		downstream: make(chan types.Event, graph.DefaultChannelBufferSize),
	}
	return &graph.Stage{
		Upstream:   s.upstream,
		Downstream: s.downstream,
		Run:        s.run,
		CloseFn:    s.close,
	}
}

func (s *turnManager) run(parent context.Context) {
	s.ctx, s.cancel = context.WithCancel(parent)
	go s.consume()
}

func (s *turnManager) consume() {
	defer close(s.downstream)
	for {
		select {
		case <-s.ctx.Done():
			return
		case evt, ok := <-s.upstream:
			if !ok {
				return
			}
			s.handle(evt)
		}
	}
}

func (s *turnManager) handle(evt types.Event) {
	switch evt.Kind {
	case types.EventVadStart:
		s.pendingFinal = ""
		s.stopSeen = false
	case types.EventVadStop:
		s.stopSeen = true
		s.emitIfReady()
	case types.EventTranscriptFinal:
		payload, ok := evt.Payload.(types.TranscriptEvent)
		if !ok {
			return
		}
		s.pendingFinal = payload.Text
		s.emitIfReady()
	}
}

func (s *turnManager) emitIfReady() {
	if !s.stopSeen {
		return
	}
	text := strings.TrimSpace(s.pendingFinal)
	if text == "" {
		return
	}
	req := types.ResponsesRequest{Role: "user", Text: text}
	select {
	case <-s.ctx.Done():
		return
	case s.downstream <- types.Event{Kind: types.EventResponsesRequest, Payload: req}:
	}
	s.pendingFinal = ""
	s.stopSeen = false
}

func (s *turnManager) close() error {
	s.once.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		close(s.upstream)
	})
	return nil
}
