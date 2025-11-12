package printer

import (
	"context"
	"errors"
	"log"

	"smart-speaker/internal/graph"
	types "smart-speaker/internal/types"
)

// Stage consumes OutputLine messages and prints them.
type Stage struct {
	printer *Printer
}

// NewStage creates a printer stage.
func NewStage() *Stage {
	return &Stage{printer: New()}
}

// Process reads outputs from upstream and renders them.
func (s *Stage) Process(ctx context.Context, upstream <-chan interface{}) <-chan interface{} {
	if upstream == nil {
		log.Printf("printer stage requires upstream input")
		return nil
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case data, ok := <-upstream:
				if !ok {
					return
				}
				line, ok := data.(types.OutputLine)
				if !ok {
					log.Printf("unexpected upstream data type: %T", data)
					continue
				}
				if err := s.printer.Process(ctx, line); err != nil {
					if errors.Is(err, context.Canceled) {
						return
					}
					log.Printf("printer stage error: %v", err)
					return
				}
			}
		}
	}()
	return nil
}

// Close flushes the printer.
func (s *Stage) Close() error {
	return s.printer.Close()
}

var _ graph.Stage = (*Stage)(nil)
