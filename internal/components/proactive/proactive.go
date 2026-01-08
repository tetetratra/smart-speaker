package proactive

import (
	"context"
	"math/rand"
	"time"

	"smart-speaker/internal/graph"
	"smart-speaker/internal/state"
	types "smart-speaker/internal/types"
)

const (
	minIdleMinutes        = 10
	maxRandomDelayMinutes = 10
	checkInterval         = time.Second
	quietHoursStart       = 23
	quietHoursEnd         = 9
)

const proactivePrompt = "独り言か軽い呼びかけを短い1文で返してください"

type runner struct {
	downstream  chan types.Event
	ctx         context.Context
	cancel      context.CancelFunc
	once        bool
	lastFiredAt time.Time
	location    *time.Location
}

// NewStage creates a proactive stage that emits system prompts based on timing conditions.
func NewStage() *graph.Stage {
	loc, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		loc = time.FixedZone("JST", 9*60*60)
	}
	r := &runner{
		downstream: make(chan types.Event, graph.DefaultChannelBufferSize),
		location:   loc,
	}
	return &graph.Stage{
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
}

func (r *runner) check() {
	present, _ := state.Get()
	lastTalkAt := state.GetLastAssistantTalkAt()
	if lastTalkAt.IsZero() {
		return
	}
	now := time.Now().In(r.location)
	scheduled := computeScheduledAt(lastTalkAt, r.location)
	if now.Before(scheduled) {
		return
	}
	if !present || isQuietHours(now) {
		state.SetLastAssistantTalkAt(now)
		return
	}
	if !r.lastFiredAt.IsZero() && !r.lastFiredAt.Before(scheduled) {
		return
	}
	r.emit(types.Event{Kind: types.EventRealtimeOutput, Payload: types.OutputLine{Role: "system", Text: proactivePrompt, Source: "proactive"}})
	r.emit(types.Event{Kind: types.EventResponsesRequest, Payload: types.ResponsesRequest{Role: "system", Text: proactivePrompt}})
	r.lastFiredAt = now
}

func computeScheduledAt(lastTalkAt time.Time, loc *time.Location) time.Time {
	seed := lastTalkAt.Unix()
	gen := rand.New(rand.NewSource(seed))
	randomDelay := time.Duration(gen.Intn(maxRandomDelayMinutes+1)) * time.Minute
	base := lastTalkAt.In(loc).Add(time.Duration(minIdleMinutes) * time.Minute)
	return base.Add(randomDelay)
}

func isQuietHours(t time.Time) bool {
	hour := t.Hour()
	return hour >= quietHoursStart || hour < quietHoursEnd
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
	return nil
}
