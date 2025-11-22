package printer

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"smart-speaker/internal/graph"
	types "smart-speaker/internal/types"
)

type printerSink struct {
	writer          *bufio.Writer
	upstream        chan types.Event
	ctx             context.Context
	cancel          context.CancelFunc
	closerWaitGroup sync.WaitGroup
	closeOnce       sync.Once
}

// NewStage builds a printer sink for the graph.
func NewStage() *graph.Stage {
	s := &printerSink{
		writer:   bufio.NewWriter(os.Stdout),
		upstream: make(chan types.Event),
	}
	return &graph.Stage{
		Upstream: s.upstream,
		Run:      s.run,
		CloseFn:  s.close,
	}
}

func (s *printerSink) run(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	s.ctx = ctx
	s.cancel = cancel
	s.closerWaitGroup.Add(1)
	go func() {
		defer s.closerWaitGroup.Done()
		for {
			select {
			case <-s.ctx.Done():
				return
			case evt, ok := <-s.upstream:
				if !ok {
					return
				}
				if evt.Kind != types.EventRealtimeOutput {
					continue
				}
				line, ok := evt.Payload.(types.OutputLine)
				if !ok {
					continue
				}
				if err := s.process(line); err != nil {
					if errors.Is(err, context.Canceled) {
						return
					}
					log.Printf("printer stage error: %v", err)
					return
				}
			}
		}
	}()
}

func (s *printerSink) close() error {
	var err error
	s.closeOnce.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		s.closerWaitGroup.Wait()
		close(s.upstream)
		if flushErr := s.writer.Flush(); flushErr != nil {
			log.Printf("flush error: %v", flushErr)
			err = flushErr
		}
		log.Println("printer: stage closed")
	})
	return err
}

func (s *printerSink) process(line types.OutputLine) error {
	if err := s.ctx.Err(); err != nil {
		return err
	}
	label := renderRoleLabel(line.Role)
	if label == "" {
		return nil
	}
	if _, err := fmt.Fprintf(s.writer, "%s: %s\n", label, line.Text); err != nil {
		return err
	}
	return s.writer.Flush()
}

func renderRoleLabel(role string) string {
	switch role {
	case "assistant":
		return "Assistant"
	case "error":
		return strings.Title(role)
	case "user":
		return ""
	default:
		if role == "" {
			return "Assistant"
		}
		return strings.Title(role)
	}
}
