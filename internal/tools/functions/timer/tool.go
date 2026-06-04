package timer

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/tetetratra/smart-speaker/internal/states/generation"
	timerstate "github.com/tetetratra/smart-speaker/internal/states/timer"
	"github.com/tetetratra/smart-speaker/internal/tools"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

const (
	toolName = "timer"

	defaultTickInterval = time.Second
)

type Config struct {
	Store        *timerstate.Store
	Generation   *generation.Store
	TickInterval time.Duration
	Now          func() time.Time
}

type Tool struct {
	store        *timerstate.Store
	generation   *generation.Store
	tickInterval time.Duration
	now          func() time.Time

	mu      sync.RWMutex
	ctx     context.Context
	emit    func(types.Event)
	started bool
}

func New(cfg Config) *Tool {
	store := cfg.Store
	if store == nil {
		store = timerstate.NewStore()
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	tickInterval := cfg.TickInterval
	if tickInterval <= 0 {
		tickInterval = defaultTickInterval
	}
	return &Tool{
		store:        store,
		generation:   cfg.Generation,
		tickInterval: tickInterval,
		now:          now,
	}
}

func (t *Tool) Name() string { return toolName }

func (t *Tool) SetContext(ctx context.Context) {
	t.mu.Lock()
	t.ctx = ctx
	t.mu.Unlock()
	t.startMonitorIfReady()
}

func (t *Tool) SetEventEmitter(emit func(types.Event)) {
	t.mu.Lock()
	t.emit = emit
	t.mu.Unlock()
	t.startMonitorIfReady()
	t.emitState()
}

func (t *Tool) Run(args map[string]any) (map[string]any, error) {
	operation := strings.TrimSpace(asString(args["operation"]))
	switch operation {
	case "create":
		return t.create(args)
	case "cancel":
		return t.cancel(args)
	default:
		return nil, fmt.Errorf("operation must be create or cancel")
	}
}

func (t *Tool) create(args map[string]any) (map[string]any, error) {
	atRaw := strings.TrimSpace(asString(args["at"]))
	if atRaw == "" {
		return nil, fmt.Errorf("at is required")
	}
	at, err := time.Parse(time.RFC3339, atRaw)
	if err != nil {
		return nil, fmt.Errorf("at must be RFC3339")
	}
	if !at.After(t.now()) {
		return nil, fmt.Errorf("at must be in the future")
	}
	action := strings.TrimSpace(asString(args["action"]))
	if action == "" {
		return nil, fmt.Errorf("action is required")
	}
	timer := t.store.Create(at, action)
	t.emitState()
	return map[string]any{
		"id":         timer.ID,
		"at":         timer.At.Format(time.RFC3339),
		"action":     timer.Action,
		"created_at": timer.CreatedAt.Format(time.RFC3339),
		"scheduled":  true,
	}, nil
}

func (t *Tool) cancel(args map[string]any) (map[string]any, error) {
	id := strings.TrimSpace(asString(args["id"]))
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}
	timer, ok := t.store.Cancel(id)
	if !ok {
		return nil, fmt.Errorf("timer not found: %s", id)
	}
	t.emitState()
	return map[string]any{
		"id":        timer.ID,
		"at":        timer.At.Format(time.RFC3339),
		"action":    timer.Action,
		"cancelled": true,
	}, nil
}

func (t *Tool) Definition() map[string]any {
	return tools.DefinitionWithMode(map[string]any{
		"type": "function",
		"name": toolName,
		"description": `未来の時刻に扱う自然言語actionをメモリ上のタイマーとして登録またはキャンセルします。
「20分後に起こして」「21時になったらエアコンをoffにして」のような依頼は、現在日時をもとに絶対時刻へ解釈し、operation=create、at=RFC3339、action=実行したい自然言語で登録してください。
期限到達時には保存したactionがAIへ通知されます。`,
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"operation": map[string]any{
					"type":        "string",
					"enum":        []string{"create", "cancel"},
					"description": "create はタイマー登録、cancel は既存タイマーの取消。",
				},
				"at": map[string]any{
					"type":        "string",
					"description": "create時に必須。期限到達時刻をRFC3339で指定します。例: 2026-06-03T21:00:00+09:00",
				},
				"action": map[string]any{
					"type":        "string",
					"description": "create時に必須。期限到達時にAIへ渡す自然言語の実行内容。",
				},
				"id": map[string]any{
					"type":        "string",
					"description": "cancel時に必須。取り消すタイマーID。",
				},
			},
			"required":             []string{"operation"},
			"additionalProperties": false,
		},
	}, tools.ToolModeWrite)
}

func (t *Tool) startMonitorIfReady() {
	t.mu.Lock()
	if t.started || t.ctx == nil || t.emit == nil {
		t.mu.Unlock()
		return
	}
	ctx := t.ctx
	t.started = true
	t.mu.Unlock()

	go t.monitor(ctx)
}

func (t *Tool) monitor(ctx context.Context) {
	ticker := time.NewTicker(t.tickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			t.emitDue(t.now())
		}
	}
}

func (t *Tool) emitDue(now time.Time) {
	due := t.store.PopDue(now)
	if len(due) == 0 {
		return
	}
	t.emitState()
	for _, timer := range due {
		generationID := types.GenerationID(0)
		if t.generation != nil {
			generationID = t.generation.Next()
		}
		text := fmt.Sprintf("タイマーの期限に到達しました。at=%s action=%s", timer.At.Format(time.RFC3339), timer.Action)
		t.emitEvent(types.Event{
			Kind: types.EventConversationCommitRequest,
			Payload: types.ConversationCommitRequest{
				Role:         types.RoleSystem,
				Text:         text,
				Source:       toolName,
				GenerationID: generationID,
			},
		})
	}
}

func (t *Tool) emitState() {
	items := t.store.Snapshot()
	state := types.TimerState{Timers: make([]types.TimerStateItem, 0, len(items))}
	for _, timer := range items {
		state.Timers = append(state.Timers, types.TimerStateItem{
			ID:        timer.ID,
			At:        timer.At,
			Action:    timer.Action,
			CreatedAt: timer.CreatedAt,
		})
	}
	t.emitEvent(types.Event{Kind: types.EventTimerState, Payload: state})
}

func (t *Tool) emitEvent(evt types.Event) {
	t.mu.RLock()
	emit := t.emit
	t.mu.RUnlock()
	if emit != nil {
		emit(evt)
	}
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

var _ tools.Handler = (*Tool)(nil)
var _ tools.ContextAware = (*Tool)(nil)
var _ tools.EventEmitterAware = (*Tool)(nil)
var _ tools.DefinitionProvider = (*Tool)(nil)
