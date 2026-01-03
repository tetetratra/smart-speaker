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
	kind := toStr(args["type"])
	desc := toStr(args["description"])
	if desc == "" {
		return nil, fmt.Errorf("description is required")
	}
	switch kind {
	case "relative":
		minutes, minutesErr := asInt(args["minutes"])
		seconds, secondsErr := asInt(args["seconds"])
		if minutesErr != nil {
			minutes = 0
		}
		if secondsErr != nil {
			seconds = 0
		}
		if minutes <= 0 && seconds <= 0 {
			return nil, fmt.Errorf("minutes or seconds must be a positive integer")
		}
		if minutes < 0 || seconds < 0 {
			return nil, fmt.Errorf("minutes and seconds must be positive integers")
		}
		delay := time.Duration(minutes)*time.Minute + time.Duration(seconds)*time.Second
		target := t.now().Add(delay)
		t.schedule(target, desc)
		result := map[string]any{
			"scheduled_for": target.Format(time.RFC3339),
			"type":          "relative",
		}
		if minutes > 0 {
			result["minutes"] = minutes
		}
		if seconds > 0 {
			result["seconds"] = seconds
		}
		return result, nil
	case "absolute":
		abs, err := parseAbsolute(args, t.now())
		if err != nil {
			return nil, err
		}
		target := time.Date(abs.year, time.Month(abs.month), abs.day, abs.hour, abs.minute, abs.second, 0, time.Local)
		now := t.now()
		if !target.After(now) {
			return nil, fmt.Errorf("specified time is not in the future")
		}
		t.schedule(target, desc)
		return map[string]any{
			"scheduled_for": target.Format(time.RFC3339),
			"type":          "absolute",
		}, nil
	default:
		return nil, fmt.Errorf("type must be 'relative' or 'absolute'")
	}
}

func (t *Tool) Definition() map[string]any {
	return map[string]any{
		"type":        "function",
		"name":        toolName,
		"description": "ユーザーが「あとで起こして」「〇時に知らせて」など、明示的にタイマーを依頼した場合のみ使用します。日付等の指定が無い場合は推測して指定してください",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"type": map[string]any{
					"type":        "string",
					"enum":        []string{"absolute", "relative"},
					"description": "absolute か relative を指定してください。ユーザーの発話に時間指定がある場合のみ使います。",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "その時間に知らせたい内容（短め）。ユーザーが明示的に依頼した内容のみ。",
				},
				"minutes": map[string]any{
					"type":        "integer",
					"description": "relative の場合、何分後か（整数）。ユーザーの発話に明示がある場合のみ。",
				},
				"seconds": map[string]any{
					"type":        "integer",
					"description": "relative の場合、何秒後か（整数）。ユーザーの発話に明示がある場合のみ。",
				},
				"hour": map[string]any{
					"type":        "integer",
					"description": "absolute の場合の時（0-23）。日付は本日固定です。",
				},
				"minute": map[string]any{
					"type":        "integer",
					"description": "absolute の場合の分（0-59）。日付は本日固定です。",
				},
				"second": map[string]any{
					"type":        "integer",
					"description": "absolute の場合の秒（0-59）。日付は本日固定です。",
				},
			},
			"required": []string{"type", "description"},
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
		text := fmt.Sprintf("タイマー: %s", desc)
		if t.emit != nil {
			t.emit(types.Event{
				Kind: types.EventTextInput,
				Payload: types.OutputLine{
					Role: "system",
					Text: text,
				},
			})
		} else {
			log.Printf("timer fired (no emitter): %s", text)
		}
	}()
}

type absoluteTime struct {
	year   int
	month  int
	day    int
	hour   int
	minute int
	second int
}

func parseAbsolute(args map[string]any, now time.Time) (absoluteTime, error) {
	if args["year"] != nil || args["month"] != nil || args["day"] != nil {
		return absoluteTime{}, fmt.Errorf("year/month/day are not supported; date defaults to today")
	}
	hour, err := asInt(args["hour"])
	if err != nil {
		return absoluteTime{}, fmt.Errorf("hour must be int")
	}
	minute, err := asInt(args["minute"])
	if err != nil {
		return absoluteTime{}, fmt.Errorf("minute must be int")
	}
	second, err := asInt(args["second"])
	if err != nil {
		second = 0
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 || second < 0 || second > 59 {
		return absoluteTime{}, fmt.Errorf("invalid date/time range")
	}
	return absoluteTime{
		year:   now.Year(),
		month:  int(now.Month()),
		day:    now.Day(),
		hour:   hour,
		minute: minute,
		second: second,
	}, nil
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
