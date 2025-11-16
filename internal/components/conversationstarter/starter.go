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

// Config defines how ConversationStarter triggers proactive prompts.
type Config struct {
	Interval time.Duration
	Prompt   string
}

// Stage emits system EventTextInput at configured intervals.
type stage struct {
	cfg        Config
	downstream chan types.Event
	ctx        context.Context
	cancel     context.CancelFunc
	lineWG     sync.WaitGroup
}

func NewStage(cfg Config) *graph.Stage {
	s := &stage{
		cfg:        cfg,
		downstream: make(chan types.Event),
	}
	return &graph.Stage{
		Downstream: s.downstream,
		Run:        s.run,
		CloseFn:    s.Close,
	}
}

func (s *stage) run(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	s.ctx = ctx
	s.cancel = cancel
	s.lineWG.Add(1)
	go func() {
		defer s.lineWG.Done()
		s.produce()
	}()
}

func (s *stage) produce() {
	defer close(s.downstream)
	ticker := time.NewTicker(s.cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			text := strings.TrimSpace(s.cfg.Prompt)
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
			select {
			case <-s.ctx.Done():
				return
			case s.downstream <- evt:
			}
		}
	}
}

func (s *stage) Close() error {
	if s.cancel != nil {
		s.cancel()
	}
	s.lineWG.Wait()
	log.Println("conversationstarter: stage closed")
	return nil
}
