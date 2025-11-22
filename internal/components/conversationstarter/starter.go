package conversationstarter

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"smart-speaker/internal/graph"
	types "smart-speaker/internal/types"
)

// Stage emits system EventTextInput at configured intervals.
type conversationStarter struct {
	interval        time.Duration
	prompt          string
	downstream      chan types.Event
	ctx             context.Context
	cancel          context.CancelFunc
	closerWaitGroup sync.WaitGroup
}

const downstreamBuffer = 16

func NewStage(interval time.Duration, prompt string) *graph.Stage {
	s := &conversationStarter{
		interval:   interval,
		prompt:     prompt,
		downstream: make(chan types.Event, downstreamBuffer),
	}
	return &graph.Stage{
		Downstream: s.downstream,
		Run:        s.run,
		CloseFn:    s.close,
	}
}

func (s *conversationStarter) run(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	s.ctx = ctx
	s.cancel = cancel
	s.closerWaitGroup.Add(1)
	go func() {
		defer s.closerWaitGroup.Done()
		s.produce()
	}()
}

func (s *conversationStarter) produce() {
	defer close(s.downstream)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			text := strings.TrimSpace(s.prompt)
			if text == "" {
				continue
			}
			evt := types.Event{
				Kind: types.EventTextInput,
				Payload: types.OutputLine{
					Role: "system",
					Text: text,
				},
			}
			s.downstream <- evt
		}
	}
}

func (s *conversationStarter) close() error {
	s.cancel()
	s.closerWaitGroup.Wait()
	log.Println("conversationstarter: stage closed")
	return nil
}
