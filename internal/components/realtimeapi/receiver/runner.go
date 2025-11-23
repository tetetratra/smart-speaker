package receiver

import (
	"context"
	"errors"
	"log"

	types "smart-speaker/internal/types"
)

// Runner pulls events from the EventStream and forwards them downstream.
type Runner struct {
	ctx        context.Context
	stream     *EventStream
	downstream chan<- types.Event
}

func NewRunner(ctx context.Context, stream *EventStream, downstream chan<- types.Event) *Runner {
	return &Runner{ctx: ctx, stream: stream, downstream: downstream}
}

func (r *Runner) Run() {
	defer close(r.downstream)
	for {
		evt, err := r.stream.Next(r.ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			log.Printf("realtime read error: %v", err)
			return
		}
		select {
		case <-r.ctx.Done():
			return
		case r.downstream <- evt:
		}
	}
}
