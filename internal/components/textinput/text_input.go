package textinput

import (
	"bufio"
	"context"
	"log"
	"os"
	"strings"
	"sync"

	"smart-speaker/internal/graph"
	types "smart-speaker/internal/types"
)

// Stage reads text from STDIN and emits EventTextInput events.
type Stage struct {
	upstream   chan types.Event
	downstream chan types.Event
	ctx        context.Context
	cancel     context.CancelFunc
	once       sync.Once
}

func NewStage(parent context.Context) *Stage {
	ctx, cancel := context.WithCancel(parent)
	s := &Stage{
		upstream:   make(chan types.Event),
		downstream: make(chan types.Event),
		ctx:        ctx,
		cancel:     cancel,
	}
	go s.drainUpstream()
	go s.produce()
	return s
}

func (s *Stage) drainUpstream() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case _, ok := <-s.upstream:
			if !ok {
				return
			}
		}
	}
}

func (s *Stage) produce() {
	defer close(s.downstream)
	scanner := bufio.NewScanner(os.Stdin)
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				log.Printf("textinput: scan error: %v", err)
			}
			return
		}
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		evt := types.Event{Kind: types.EventTextInput, Payload: types.OutputLine{Role: "user", Text: text}}
		select {
		case <-s.ctx.Done():
			return
		case s.downstream <- evt:
		}
	}
}

func (s *Stage) Upstream() chan<- types.Event { return s.upstream }

func (s *Stage) Downstream() <-chan types.Event { return s.downstream }

func (s *Stage) Close() error {
	s.once.Do(func() {
		s.cancel()
		close(s.upstream)
	})
	return nil
}

var _ graph.Stage = (*Stage)(nil)
