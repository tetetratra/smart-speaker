package sessionlifecycle

import (
	"context"
	"strings"
	"sync"
	"time"

	"smart-speaker/internal/graph"
	types "smart-speaker/internal/types"
)

const (
	defaultIdleThreshold = 60 * time.Minute
	diaryPrompt          = `今回の会話を日記としてまとめ、write_diary ツールを呼び出して書いてください。
すでに過去の日記に書かれていることではなく、今回の会話で話した内容にのみ触れてください。`
	shortDiaryInstruction  = "今回の会話は短いため、日記は1行程度で簡潔に書いてください。"
	mediumDiaryInstruction = "今回の会話は中程度の長さのため、日記は2〜3行程度で書いてください。"
	longDiaryInstruction   = "今回の会話は長いため、日記は3〜5行程度で必要な内容をまとめてください。"
)

type Config struct {
	WriteDiaryTools []any
	IdleThreshold   time.Duration
}

type state struct {
	lastActivityAt time.Time
	snapshot       []types.ChatMessage
	diaryInFlight  bool
}

type runner struct {
	upstream   chan types.Event
	downstream chan types.Event
	tools      []any

	idleThreshold time.Duration

	ctx    context.Context
	cancel context.CancelFunc
	once   sync.Once

	state  state
	timer  *time.Timer
	timerC <-chan time.Time
}

func NewStage(cfg Config) *graph.Stage {
	idleThreshold := cfg.IdleThreshold
	if idleThreshold <= 0 {
		idleThreshold = defaultIdleThreshold
	}
	r := &runner{
		upstream:      make(chan types.Event, graph.DefaultChannelBufferSize),
		downstream:    make(chan types.Event, graph.DefaultChannelBufferSize),
		tools:         cfg.WriteDiaryTools,
		idleThreshold: idleThreshold,
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
			r.timer = nil
			r.timerC = nil
			r.handleIdleTimeout()
		}
	}
}

func (r *runner) handleEvent(evt types.Event) {
	switch evt.Kind {
	case types.EventConversationSnapshotUpdated:
		snapshot, ok := evt.Payload.(types.ConversationSnapshot)
		if !ok {
			return
		}
		r.state.snapshot = cloneMessages(snapshot.Messages)
	case types.EventConversationActivity:
		activity, ok := evt.Payload.(types.ConversationActivity)
		if !ok {
			return
		}
		r.handleActivity(activity)
	case types.EventToolResponse:
		resp, ok := evt.Payload.(types.ToolResponse)
		if !ok {
			return
		}
		r.handleToolResponse(resp)
	}
}

func (r *runner) handleActivity(activity types.ConversationActivity) {
	at := activity.At
	if at.IsZero() {
		at = time.Now()
	}
	r.state.lastActivityAt = at
	r.state.diaryInFlight = false
	r.startTimer(time.Until(at.Add(r.idleThreshold)))
}

func (r *runner) handleToolResponse(resp types.ToolResponse) {
	if strings.TrimSpace(resp.Name) != "write_diary" {
		return
	}
	if !r.state.diaryInFlight {
		return
	}
	r.state.diaryInFlight = false
	r.resetState()
	r.emit(types.Event{Kind: types.EventSessionClear})
}

func (r *runner) handleIdleTimeout() {
	if r.state.diaryInFlight {
		return
	}
	if len(r.state.snapshot) == 0 || len(r.tools) == 0 {
		return
	}
	if r.state.lastActivityAt.IsZero() {
		return
	}
	if remaining := time.Until(r.state.lastActivityAt.Add(r.idleThreshold)); remaining > 0 {
		r.startTimer(remaining)
		return
	}
	r.state.diaryInFlight = true
	emptySystem := ""
	r.emit(types.Event{
		Kind: types.EventResponsesRequest,
		Payload: types.ResponsesRequest{
			Role:         "system",
			Text:         buildDiaryPrompt(r.state.snapshot),
			Messages:     cloneMessages(r.state.snapshot),
			SystemPrompt: &emptySystem,
			ToolChoice:   map[string]any{"type": "function", "name": "write_diary"},
			Tools:        r.tools,
		},
	})
}

func buildDiaryPrompt(messages []types.ChatMessage) string {
	instruction := diaryLengthInstruction(messages)
	if instruction == "" {
		return diaryPrompt
	}
	return diaryPrompt + "\n" + instruction
}

func diaryLengthInstruction(messages []types.ChatMessage) string {
	count := conversationMessageCount(messages)
	switch {
	case count <= 0:
		return ""
	case count <= 6:
		return shortDiaryInstruction
	case count <= 12:
		return mediumDiaryInstruction
	default:
		return longDiaryInstruction
	}
}

func conversationMessageCount(messages []types.ChatMessage) int {
	count := 0
	for _, msg := range messages {
		switch strings.TrimSpace(msg.Role) {
		case "user", "assistant":
			if strings.TrimSpace(msg.Content) != "" {
				count++
			}
		}
	}
	return count
}

func (r *runner) startTimer(d time.Duration) {
	if d <= 0 {
		r.stopTimer()
		r.handleIdleTimeout()
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

func (r *runner) resetState() {
	r.stopTimer()
	r.state = state{}
}

func (r *runner) emit(evt types.Event) {
	select {
	case <-r.ctx.Done():
		return
	case r.downstream <- evt:
	}
}

func (r *runner) close() error {
	r.once.Do(func() {
		if r.cancel != nil {
			r.cancel()
		}
		close(r.upstream)
	})
	return nil
}

func cloneMessages(messages []types.ChatMessage) []types.ChatMessage {
	if len(messages) == 0 {
		return nil
	}
	out := make([]types.ChatMessage, len(messages))
	copy(out, messages)
	return out
}
