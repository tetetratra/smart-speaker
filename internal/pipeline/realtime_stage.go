package pipeline

import (
	"context"
	"errors"
	"log"

	"smart-speaker/internal/components/realtimeapi"
	types "smart-speaker/internal/types"
)

// RealtimeStage bridges audio input to the Realtime API and emits assistant outputs.
type RealtimeStage struct {
	client *realtimeapi.Client
}

func NewRealtimeStage(ctx context.Context, cfg realtimeapi.Config) (*RealtimeStage, error) {
	client, err := realtimeapi.NewClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &RealtimeStage{client: client}, nil
}

func (r *RealtimeStage) Process(ctx context.Context, upstream <-chan interface{}) <-chan interface{} {
	if upstream == nil {
		log.Printf("realtime stage requires an upstream source")
		return nil
	}
	out := make(chan interface{})

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
				if err := r.client.Process(ctx, chunk); err != nil {
					if errors.Is(err, context.Canceled) {
						return
					}
					log.Printf("realtime send error: %v", err)
					return
				}
			}
		}
	}()

	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			line, err := r.client.Read(ctx)
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

func (r *RealtimeStage) Close() error {
	return r.client.Close()
}
