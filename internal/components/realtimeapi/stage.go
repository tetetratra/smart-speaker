package realtimeapi

import (
	"context"
	"errors"
	"log"

	"smart-speaker/internal/graph"
	types "smart-speaker/internal/types"
)

// Stage wraps the Realtime API client into a graph stage.
type Stage struct {
	client *Client
}

// NewStage constructs a realtime stage with the given config.
func NewStage(ctx context.Context, cfg Config) (*Stage, error) {
	client, err := NewClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &Stage{client: client}, nil
}

// Process consumes audio chunks from upstream and emits assistant outputs.
func (s *Stage) Process(ctx context.Context, upstream <-chan interface{}) <-chan interface{} {
	if upstream == nil {
		log.Printf("realtime stage requires upstream input")
		return nil
	}

	out := make(chan interface{})

	go func() {
		defer close(out)

		// sender
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case data, ok := <-upstream:
					if !ok {
						return
					}
					chunk, ok := data.(types.AudioChunk)
					if !ok {
						log.Printf("unexpected upstream data type: %T", data)
						continue
					}
					if err := s.client.Process(ctx, chunk); err != nil {
						if errors.Is(err, context.Canceled) {
							return
						}
						log.Printf("realtime send error: %v", err)
						return
					}
				}
			}
		}()

		for {
			line, err := s.client.Read(ctx)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				log.Printf("realtime read error: %v", err)
				return
			}
			select {
			case <-ctx.Done():
				return
			case out <- line:
			}
		}
	}()

	return out
}

// Close closes the underlying client.
func (s *Stage) Close() error {
	return s.client.Close()
}

var _ graph.Stage = (*Stage)(nil)
