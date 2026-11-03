package audiopace

import (
	"context"
	"github.com/tetetratra/smart-speaker/internal/graph"
	"github.com/tetetratra/smart-speaker/internal/types"
	"time"
)

// NewStage は一フレーム分だけ待つ。旧世代の累積送信予定時刻を保持しない。
func NewStage() *graph.Stage {
	st := &graph.Stage{Name: "audiopace", Upstream: make(chan types.Event), Downstream: make(chan types.Event), CloseFn: func() error { return nil }}
	st.Run = func(ctx context.Context) {
		defer close(st.Downstream)
		for {
			e, ok := st.Receive(ctx)
			if !ok {
				return
			}
			p, ok := e.Payload.(types.AudioFrame)
			if !ok {
				continue
			}
			timer := time.NewTimer(p.Duration)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			select {
			case <-ctx.Done():
				return
			case st.Downstream <- e:
			}
		}
	}
	return st
}
