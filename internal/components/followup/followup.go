package followup

import (
	"context"
	"time"

	"smart-speaker/internal/graph"
	types "smart-speaker/internal/types"
)

const followupPrompt = "会話を続ける短い一文を出力してください。この返答に付与するexpectationは基本的には0を指定してください"

type runner struct {
	upstream   chan types.Event
	downstream chan types.Event
	ctx        context.Context
	cancel     context.CancelFunc
	once       bool

	timer            *time.Timer
	timerC           <-chan time.Time
	lastExpectation  int
	readyForFollowup bool
	pendingRespID    string
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
			r.readyForFollowup = false
			return
		case evt, ok := <-r.upstream:
			if !ok {
				r.stopTimer()
				r.readyForFollowup = false
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
		if line.Text != "" {
			r.pendingRespID = line.ResponseID
		}
		if !line.Final {
			return
		}
		r.readyForFollowup = true
	case types.EventTextInput:
		r.stopTimer()
		r.readyForFollowup = false
	case types.EventTTSEnd:
		if !r.readyForFollowup {
			return
		}
		tts, ok := evt.Payload.(types.TTSEvent)
		if !ok {
			return
		}
		if tts.ResponseID == "" || r.pendingRespID == "" || tts.ResponseID != r.pendingRespID {
			return
		}
		r.readyForFollowup = false
		expectation := r.lastExpectation
		if expectation <= 0 {
			r.stopTimer()
			return
		}
		r.startTimer(expectationDelay(expectation))
	}
}

func expectationDelay(expectation int) time.Duration {
	switch expectation {
	case 1:
		return 5 * time.Second
	case 2:
		return 10 * time.Second
	default:
		return 0
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
