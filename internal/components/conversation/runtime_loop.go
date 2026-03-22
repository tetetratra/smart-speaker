package conversation

import (
	"context"
	"log"

	types "smart-speaker/internal/types"
)

func (r *runner) run(parent context.Context) {
	r.ctx, r.cancel = context.WithCancel(parent)
	go r.consume()
}

func (r *runner) consume() {
	defer close(r.downstream)
	for {
		select {
		case <-r.ctx.Done():
			r.stopTimer()
			return
		case evt, ok := <-r.upstream:
			if !ok {
				r.stopTimer()
				return
			}
			r.handleEvent(evt)
		case <-r.timerC:
			r.timerC = nil
			r.timer = nil
			r.applyEffects(r.core.Handle(timerElapsedSignal{}))
		}
	}
}

func (r *runner) handleEvent(evt types.Event) {
	sig, ok := signalFromEvent(evt)
	if !ok {
		return
	}
	r.applyEffects(r.core.Handle(sig))
}

func (r *runner) close() error {
	if r.once {
		return nil
	}
	r.once = true
	if r.cancel != nil {
		r.cancel()
	}
	if err := r.logger.Close(); err != nil {
		log.Printf("conversation: log close error: %v", err)
	}
	close(r.upstream)
	return nil
}
