package audiopace

import (
	"context"
	"github.com/tetetratra/smart-speaker/internal/graph"
	"github.com/tetetratra/smart-speaker/internal/types"
	"time"
)

// NewStage は受け入れた一フレームずつ送信時刻を進める。
// 待機音声全体の予定を積み上げず、タイマー処理の遅れも毎フレーム累積させない。
func NewStage() *graph.Stage {
	st := &graph.Stage{Name: "audiopace", Upstream: make(chan types.Event), Downstream: make(chan types.Event), CloseFn: func() error { return nil }}
	st.Run = func(ctx context.Context) {
		defer close(st.Downstream)
		var next time.Time
		for {
			e, ok := st.Receive(ctx)
			if !ok {
				return
			}
			p, ok := e.Payload.(types.AudioFrame)
			if !ok {
				continue
			}
			now := time.Now()
			if next.IsZero() || now.Sub(next) > 100*time.Millisecond {
				next = now
			}
			// Opusの末尾パディングも含め、20ms未満の間隔で音声を先送りしない。
			duration := max(p.Duration, 20*time.Millisecond)
			if p.End {
				duration = 0
			}
			next = next.Add(duration)
			timer := time.NewTimer(max(0, time.Until(next)))
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
