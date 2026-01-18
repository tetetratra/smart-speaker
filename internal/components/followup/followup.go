package followup

import (
	"context"
	"time"

	"smart-speaker/internal/graph"
	types "smart-speaker/internal/types"
)

const followupPrompt = "回答を促す短い一文を出力してください"

type runner struct {
	upstream   chan types.Event
	downstream chan types.Event
	ctx        context.Context
	cancel     context.CancelFunc
	once       bool

	timer           *time.Timer
	timerC          <-chan time.Time
	lastExpectation int
}

// NewStage は無応答時にフォローアップの system メッセージを送るステージを作成します。
func NewStage() *graph.Stage {
	r := &runner{
		upstream:        make(chan types.Event, graph.DefaultChannelBufferSize),
		downstream:      make(chan types.Event, graph.DefaultChannelBufferSize),
		lastExpectation: -1,
	}
	return &graph.Stage{
		Upstream:   r.upstream,
		Downstream: r.downstream,
		Run:        r.run,
		CloseFn:    r.close,
	}
}

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
			r.emit(types.Event{
				Kind: types.EventResponsesRequest,
				Payload: types.ResponsesRequest{
					Role: "system",
					Text: followupPrompt,
				},
			})
		}
	}
}

func (r *runner) handleEvent(evt types.Event) {
	switch evt.Kind {
	case types.EventRealtimeOutput:
		line, ok := evt.Payload.(types.OutputLine)
		if !ok {
			return
		}
		if line.Role != "assistant" {
			return
		}
		if line.Expectation != nil {
			r.lastExpectation = *line.Expectation
		}
		if !line.Final {
			return
		}
		expectation := r.lastExpectation
		if line.Expectation != nil {
			expectation = *line.Expectation
		}
		if expectation <= 0 {
			r.stopTimer()
			return
		}
		r.startTimer(time.Duration(expectation) * time.Second)
	case types.EventTextInput:
		r.stopTimer()
	}
}

func (r *runner) startTimer(d time.Duration) {
	if d <= 0 {
		r.stopTimer()
		return
	}
	if r.timer == nil {
		r.timer = time.NewTimer(d)
		r.timerC = r.timer.C
		return
	}
	if !r.timer.Stop() {
		select {
		case <-r.timer.C:
		default:
		}
	}
	r.timer.Reset(d)
	r.timerC = r.timer.C
}

func (r *runner) stopTimer() {
	if r.timer == nil {
		return
	}
	if !r.timer.Stop() {
		select {
		case <-r.timer.C:
		default:
		}
	}
	r.timer = nil
	r.timerC = nil
}

func (r *runner) emit(evt types.Event) {
	select {
	case <-r.ctx.Done():
		return
	case r.downstream <- evt:
	}
}

func (r *runner) close() error {
	if r.once {
		return nil
	}
	r.once = true
	if r.cancel != nil {
		r.cancel()
	}
	close(r.upstream)
	return nil
}
