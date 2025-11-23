package sender

import (
	"context"

	types "smart-speaker/internal/types"
)

// Client defines the methods required by the sender runner.
type Client interface {
	Process(context.Context, types.AudioChunk) error
	Send(any) error
}

// Runner pulls events from the upstream channel and forwards them to the Realtime API.
type Runner struct {
	ctx      context.Context
	upstream <-chan types.Event
	handler  *EventHandler
}

func NewRunner(ctx context.Context, client Client, upstream <-chan types.Event, voice string) *Runner {
	return &Runner{
		ctx:      ctx,
		upstream: upstream,
		handler:  NewEventHandler(ctx, client, voice),
	}
}

func (r *Runner) Run() {
	for {
		select {
		case <-r.ctx.Done():
			return
		case evt, ok := <-r.upstream:
			if !ok {
				return
			}
			r.handler.Handle(evt)
		}
	}
}
