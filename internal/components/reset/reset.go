package reset

import (
	"context"
	"time"

	"smart-speaker/internal/graph"
	"smart-speaker/internal/state"
	types "smart-speaker/internal/types"
)

const (
	checkInterval = time.Minute
	idleThreshold = 30 * time.Minute
)

const diaryPrompt = "直近の会話を日記としてまとめ、write_diary ツールを呼び出してください"

type runner struct {
	upstream   chan types.Event
	downstream chan types.Event
	ctx        context.Context
	cancel     context.CancelFunc
	once       bool
}

// NewStage creates a stage that requests diary writing and resets response state after inactivity.
func NewStage() *graph.Stage {
	r := &runner{
		upstream:   make(chan types.Event, graph.DefaultChannelBufferSize),
		downstream: make(chan types.Event, graph.DefaultChannelBufferSize),
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
	ticker := time.NewTicker(checkInterval)
	go func() {
		defer func() {
			ticker.Stop()
			close(r.downstream)
		}()
		for {
			select {
			case <-r.ctx.Done():
				return
			case <-ticker.C:
				r.check()
			}
		}
	}()
	go r.consume()
}

func (r *runner) consume() {
	for {
		select {
		case <-r.ctx.Done():
			return
		case evt, ok := <-r.upstream:
			if !ok {
				return
			}
			if evt.Kind == types.EventSessionReset {
				r.runReset()
			}
		}
	}
}

func (r *runner) check() {
	lastActivity := state.GetLastActivityAt()
	if lastActivity.IsZero() {
		return
	}
	if time.Since(lastActivity) < idleThreshold {
		return
	}
	if state.IsDiaryWrittenSince(lastActivity) {
		return
	}
	r.runReset()
}

func (r *runner) runReset() {
	r.emit(types.Event{Kind: types.EventRealtimeOutput, Payload: types.OutputLine{Role: "system", Text: diaryPrompt, Source: "reset"}})
	r.emit(types.Event{Kind: types.EventSessionReset})
	r.emit(types.Event{Kind: types.EventResponsesRequest, Payload: types.ResponsesRequest{Role: "system", Text: diaryPrompt, ToolChoice: map[string]any{"type": "function", "name": "write_diary"}}})
	state.SetDiaryWrittenAt(time.Now())
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
