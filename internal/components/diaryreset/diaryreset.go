package diaryreset

import (
	"context"
	"os"
	"time"

	"smart-speaker/internal/graph"
)

const responseIDFile = "tmp/response_id.txt"

// NewStage resets response state at midnight JST.
func NewStage() *graph.Stage {
	r := &runner{}
	return &graph.Stage{
		Run:     r.run,
		CloseFn: r.close,
	}
}

type runner struct {
	ctx    context.Context
	cancel context.CancelFunc
	once   bool
	loc    *time.Location
}

func (r *runner) run(parent context.Context) {
	r.ctx, r.cancel = context.WithCancel(parent)
	loc, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		loc = time.FixedZone("JST", 9*60*60)
	}
	r.loc = loc
	go r.loop()
}

func (r *runner) loop() {
	for {
		select {
		case <-r.ctx.Done():
			return
		case <-time.After(r.untilNextMidnight()):
			r.reset()
		}
	}
}

func (r *runner) untilNextMidnight() time.Duration {
	now := time.Now().In(r.loc)
	next := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, r.loc).Add(24 * time.Hour)
	return time.Until(next)
}

func (r *runner) reset() {
	_ = os.Remove(responseIDFile)
}

func (r *runner) close() error {
	if r.once {
		return nil
	}
	r.once = true
	if r.cancel != nil {
		r.cancel()
	}
	return nil
}
