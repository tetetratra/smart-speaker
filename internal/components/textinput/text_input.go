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

type stage struct {
	upstream   chan types.Event
	downstream chan types.Event
	ctx        context.Context
	cancel     context.CancelFunc
	once       sync.Once
	lineWG     sync.WaitGroup
}

// NewStage reads text from STDIN and emits EventTextInput events.
func NewStage() *graph.Stage {
	s := &stage{
		upstream:   make(chan types.Event),
		downstream: make(chan types.Event),
	}
	return &graph.Stage{
		Upstream:   s.upstream,
		Downstream: s.downstream,
		Run:        s.run,
		CloseFn:    s.Close,
	}
}

func (s *stage) run(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	s.ctx = ctx
	s.cancel = cancel
	s.lineWG.Add(2)
	go func() {
		defer s.lineWG.Done()
		s.drainUpstream()
	}()
	go func() {
		defer s.lineWG.Done()
		s.produce()
	}()
}

func (s *stage) drainUpstream() {
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

func (s *stage) produce() {
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

func (s *stage) Close() error {
	s.once.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		s.lineWG.Wait()
		close(s.upstream)
		log.Println("textinput: stage closed")
	})
	return nil
}
