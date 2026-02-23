package timer

import (
	"context"
	"fmt"
	"log"
	"time"

	"smart-speaker/internal/tools"
	types "smart-speaker/internal/types"
)

const toolName = "schedule_timer"

// Tool は指定時刻にリマインドテキストを送る簡易スケジューラです。
type Tool struct {
	ctx     context.Context
	emit    func(types.Event)
	nowFunc func() time.Time
}

func New() *Tool {
	return &Tool{nowFunc: time.Now}
}

func (t *Tool) Name() string { return toolName }

func (t *Tool) SetContext(ctx context.Context) {
	t.ctx = ctx
}

func (t *Tool) SetEventEmitter(emit func(types.Event)) {
	t.emit = emit
}

func (t *Tool) Run(args map[string]any) (map[string]any, error) {
	desc := toStr(args["reminder_text"])
	if desc == "" {
		return nil, fmt.Errorf("reminder_text is required")
	}
	seconds, err := asInt(args["seconds"])
	if err != nil {
		return nil, fmt.Errorf("seconds must be an integer")
	}
	if seconds <= 0 {
		return nil, fmt.Errorf("seconds must be a positive integer")
	}
	delay := time.Duration(seconds) * time.Second
	target := t.now().Add(delay)
	t.schedule(target, desc)
	return map[string]any{
		"scheduled_for": target.Format(time.RFC3339),
		"seconds":       seconds,
	}, nil
}

func (t *Tool) Definition() map[string]any {
	return map[string]any{
		"type":        "function",
		"name":        toolName,
		"description": "あなたが自身の作業を予約するために使うツールです。指定時間後にタイマーが発動します。ユーザーが明示的に操作の要求をしたときのみ呼び出すこと。reminder_text にはタイマー発火時にあなたがやることを入れてください。タイマー通知文を受けて再度このツールを呼び出してはいけません。",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"reminder_text": map[string]any{
					"type":        "string",
					"description": "タイマー発火時にあなたがやること。例: 「起こす」, 「お茶を入れるよう伝える」, 「薬を飲むよう促す」など",
				},
				"seconds": map[string]any{
					"type":        "integer",
					"description": "何秒後か（整数）",
				},
			},
			"required": []string{"seconds", "reminder_text"},
		},
	}
}

func (t *Tool) now() time.Time {
	if t.nowFunc != nil {
		return t.nowFunc()
	}
	return time.Now()
}

func (t *Tool) schedule(target time.Time, desc string) {
	delay := time.Until(target)
	if delay <= 0 {
		return
	}
	ctx := t.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		text := fmt.Sprintf("タイマーが発火しました: %s", desc)
		if t.emit != nil {
			t.emit(types.Event{
				Kind: types.EventTimerFired,
				Payload: types.TimerFiredEvent{
					ReminderText: text,
				},
			})
		} else {
			log.Printf("timer fired (no emitter): %s", text)
		}
	}()
}

func toStr(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func asInt(v any) (int, error) {
	switch val := v.(type) {
	case float64:
		return int(val), nil
	case int:
		return val, nil
	case int32:
		return int(val), nil
	case int64:
		return int(val), nil
	default:
		return 0, fmt.Errorf("not an integer")
	}
}

var _ tools.Handler = (*Tool)(nil)
var _ tools.ContextAware = (*Tool)(nil)
var _ tools.EventEmitterAware = (*Tool)(nil)
var _ tools.DefinitionProvider = (*Tool)(nil)
