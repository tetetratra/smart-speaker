package googlecalendar

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	calendarapi "smart-speaker/internal/googlecalendar"
	"smart-speaker/internal/tools"
)

const (
	listToolName   = "google_calendar_list"
	createToolName = "google_calendar_create"
	updateToolName = "google_calendar_update"
)

var defaultLocation = time.FixedZone("Asia/Tokyo", 9*60*60)

type calendarClient interface {
	ListEvents(ctx context.Context, req calendarapi.ListEventsRequest) ([]calendarapi.Event, error)
	CreateEvent(ctx context.Context, req calendarapi.CreateEventRequest) (calendarapi.Event, error)
	UpdateEvent(ctx context.Context, req calendarapi.UpdateEventRequest) (calendarapi.Event, error)
	DeleteEvent(ctx context.Context, req calendarapi.DeleteEventRequest) error
}

type baseTool struct {
	client calendarClient
	ctx    context.Context
}

func newBaseTool(client calendarClient) *baseTool {
	if client == nil {
		client = calendarapi.NewClient(calendarapi.Config{})
	}
	return &baseTool{client: client}
}

func (t *baseTool) SetContext(ctx context.Context) {
	t.ctx = ctx
}

func (t *baseTool) ctxOrBackground() context.Context {
	if t.ctx != nil {
		return t.ctx
	}
	return context.Background()
}

type listTool struct {
	*baseTool
}

func NewList(client calendarClient) *listTool {
	return &listTool{baseTool: newBaseTool(client)}
}

func (t *listTool) Name() string { return listToolName }

func (t *listTool) Run(args map[string]any) (map[string]any, error) {
	calendarID := strings.TrimSpace(asString(args["calendar_id"]))
	if calendarID == "" {
		calendarID = "primary"
	}
	timeMinRaw := strings.TrimSpace(asString(args["time_min"]))
	timeMaxRaw := strings.TrimSpace(asString(args["time_max"]))
	dateRaw := strings.TrimSpace(asString(args["date"]))
	maxResults, _ := asInt(args["max_results"])
	if maxResults <= 0 {
		maxResults = 20
	}

	var timeMin, timeMax time.Time
	if timeMinRaw != "" || timeMaxRaw != "" {
		if timeMinRaw == "" || timeMaxRaw == "" {
			return nil, fmt.Errorf("time_min と time_max は両方指定してください")
		}
		minParsed, err := parseTimeOrDate(timeMinRaw)
		if err != nil {
			return nil, fmt.Errorf("time_min はRFC3339またはYYYY-MM-DDで指定してください")
		}
		maxParsed, err := parseTimeOrDate(timeMaxRaw)
		if err != nil {
			return nil, fmt.Errorf("time_max はRFC3339またはYYYY-MM-DDで指定してください")
		}
		timeMin = minParsed
		timeMax = maxParsed
	} else if dateRaw != "" {
		start, err := time.ParseInLocation("2006-01-02", dateRaw, defaultLocation)
		if err != nil {
			return nil, fmt.Errorf("date はYYYY-MM-DDで指定してください")
		}
		timeMin = start
		timeMax = start.Add(24 * time.Hour)
	} else {
		return nil, fmt.Errorf("date もしくは time_min/time_max を指定してください")
	}

	events, err := t.client.ListEvents(t.ctxOrBackground(), calendarapi.ListEventsRequest{
		CalendarID: calendarID,
		TimeMin:    timeMin,
		TimeMax:    timeMax,
		MaxResults: maxResults,
	})
	if err != nil {
		return nil, err
	}

	items := make([]map[string]any, 0, len(events))
	for _, item := range events {
		items = append(items, map[string]any{
			"id":          item.ID,
			"summary":     item.Summary,
			"description": item.Description,
			"location":    item.Location,
			"html_link":   item.HTMLLink,
			"start":       renderEventTime(item.Start),
			"end":         renderEventTime(item.End),
		})
	}
	return map[string]any{
		"calendar_id": calendarID,
		"time_min":    timeMin.Format(time.RFC3339),
		"time_max":    timeMax.Format(time.RFC3339),
		"items":       items,
	}, nil
}

func (t *listTool) Definition() map[string]any {
	return map[string]any{
		"type":        "function",
		"name":        listToolName,
		"description": "Googleカレンダーの予定一覧を取得します。date か time_min/time_max を指定してください。取得した情報は基本的に画面のボードにも記載してください",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"calendar_id": map[string]any{
					"type":        "string",
					"description": "カレンダーID。省略時は primary。",
				},
				"date": map[string]any{
					"type":        "string",
					"description": "YYYY-MM-DD。指定した日付の予定を取得する。",
				},
				"time_min": map[string]any{
					"type":        "string",
					"description": "RFC3339 形式の開始時刻。例: 2026-02-14T09:00:00+09:00",
				},
				"time_max": map[string]any{
					"type":        "string",
					"description": "RFC3339 形式の終了時刻。例: 2026-02-14T18:00:00+09:00",
				},
				"max_results": map[string]any{
					"type":        "integer",
					"description": "最大取得件数。省略時は20。",
				},
			},
		},
	}
}

type createTool struct {
	*baseTool
}

func NewCreate(client calendarClient) *createTool {
	return &createTool{baseTool: newBaseTool(client)}
}

func (t *createTool) Name() string { return createToolName }

func (t *createTool) Run(args map[string]any) (map[string]any, error) {
	calendarID := strings.TrimSpace(asString(args["calendar_id"]))
	if calendarID == "" {
		calendarID = "primary"
	}
	summary := strings.TrimSpace(asString(args["summary"]))
	if summary == "" {
		return nil, fmt.Errorf("summary は必須です")
	}
	startRaw := strings.TrimSpace(asString(args["start_time"]))
	endRaw := strings.TrimSpace(asString(args["end_time"]))
	if startRaw == "" || endRaw == "" {
		return nil, fmt.Errorf("start_time と end_time は必須です")
	}
	startTime, err := buildEventTime(startRaw)
	if err != nil {
		return nil, err
	}
	endTime, err := buildEventTime(endRaw)
	if err != nil {
		return nil, err
	}

	event, err := t.client.CreateEvent(t.ctxOrBackground(), calendarapi.CreateEventRequest{
		CalendarID:  calendarID,
		Summary:     summary,
		Description: strings.TrimSpace(asString(args["description"])),
		Location:    strings.TrimSpace(asString(args["location"])),
		Start:       startTime,
		End:         endTime,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"id":        event.ID,
		"summary":   event.Summary,
		"html_link": event.HTMLLink,
		"start":     renderEventTime(event.Start),
		"end":       renderEventTime(event.End),
	}, nil
}

func (t *createTool) Definition() map[string]any {
	return map[string]any{
		"type":        "function",
		"name":        createToolName,
		"description": "Googleカレンダーに予定を追加します。start_time/end_time は RFC3339 か YYYY-MM-DD。",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"calendar_id": map[string]any{
					"type":        "string",
					"description": "カレンダーID。省略時は primary。",
				},
				"summary": map[string]any{
					"type":        "string",
					"description": "予定タイトル。",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "予定の説明。",
				},
				"location": map[string]any{
					"type":        "string",
					"description": "場所。",
				},
				"start_time": map[string]any{
					"type":        "string",
					"description": "RFC3339 形式の開始時刻、またはYYYY-MM-DD。",
				},
				"end_time": map[string]any{
					"type":        "string",
					"description": "RFC3339 形式の終了時刻、またはYYYY-MM-DD。",
				},
			},
			"required": []string{"summary", "start_time", "end_time"},
		},
	}
}

type updateTool struct {
	*baseTool
}

func NewUpdate(client calendarClient) *updateTool {
	return &updateTool{baseTool: newBaseTool(client)}
}

func (t *updateTool) Name() string { return updateToolName }

func (t *updateTool) Run(args map[string]any) (map[string]any, error) {
	action := strings.TrimSpace(asString(args["action"]))
	if action == "" {
		action = "update"
	}
	eventID := strings.TrimSpace(asString(args["event_id"]))
	if eventID == "" {
		return nil, fmt.Errorf("event_id は必須です")
	}
	calendarID := strings.TrimSpace(asString(args["calendar_id"]))
	if calendarID == "" {
		calendarID = "primary"
	}
	ctx := t.ctxOrBackground()

	if action == "delete" {
		if err := t.client.DeleteEvent(ctx, calendarapi.DeleteEventRequest{CalendarID: calendarID, EventID: eventID}); err != nil {
			return nil, err
		}
		return map[string]any{
			"deleted":     true,
			"event_id":    eventID,
			"calendar_id": calendarID,
		}, nil
	}

	var summaryPtr, descPtr, locPtr *string
	if summary := strings.TrimSpace(asString(args["summary"])); summary != "" {
		summaryPtr = &summary
	}
	if desc := strings.TrimSpace(asString(args["description"])); desc != "" {
		descPtr = &desc
	}
	if loc := strings.TrimSpace(asString(args["location"])); loc != "" {
		locPtr = &loc
	}

	var startPtr, endPtr *calendarapi.EventTime
	if startRaw := strings.TrimSpace(asString(args["start_time"])); startRaw != "" {
		startTime, err := buildEventTime(startRaw)
		if err != nil {
			return nil, err
		}
		startPtr = &startTime
	}
	if endRaw := strings.TrimSpace(asString(args["end_time"])); endRaw != "" {
		endTime, err := buildEventTime(endRaw)
		if err != nil {
			return nil, err
		}
		endPtr = &endTime
	}
	if summaryPtr == nil && descPtr == nil && locPtr == nil && startPtr == nil && endPtr == nil {
		return nil, fmt.Errorf("update の場合は更新内容を指定してください")
	}

	event, err := t.client.UpdateEvent(ctx, calendarapi.UpdateEventRequest{
		CalendarID:  calendarID,
		EventID:     eventID,
		Summary:     summaryPtr,
		Description: descPtr,
		Location:    locPtr,
		Start:       startPtr,
		End:         endPtr,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"id":        event.ID,
		"summary":   event.Summary,
		"html_link": event.HTMLLink,
		"start":     renderEventTime(event.Start),
		"end":       renderEventTime(event.End),
	}, nil
}

func (t *updateTool) Definition() map[string]any {
	return map[string]any{
		"type":        "function",
		"name":        updateToolName,
		"description": "Googleカレンダーの予定を更新または削除します。",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"description": "update または delete。省略時は update。",
				},
				"calendar_id": map[string]any{
					"type":        "string",
					"description": "カレンダーID。省略時は primary。",
				},
				"event_id": map[string]any{
					"type":        "string",
					"description": "更新・削除対象のイベントID。",
				},
				"summary": map[string]any{
					"type":        "string",
					"description": "予定タイトル。",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "予定の説明。",
				},
				"location": map[string]any{
					"type":        "string",
					"description": "場所。",
				},
				"start_time": map[string]any{
					"type":        "string",
					"description": "RFC3339 形式の開始時刻、またはYYYY-MM-DD。",
				},
				"end_time": map[string]any{
					"type":        "string",
					"description": "RFC3339 形式の終了時刻、またはYYYY-MM-DD。",
				},
			},
			"required": []string{"event_id"},
		},
	}
}

func renderEventTime(t calendarapi.EventTime) map[string]any {
	out := map[string]any{}
	if t.DateTime != "" {
		out["date_time"] = t.DateTime
	}
	if t.Date != "" {
		out["date"] = t.Date
	}
	if t.TimeZone != "" {
		out["time_zone"] = t.TimeZone
	}
	return out
}

func buildEventTime(raw string) (calendarapi.EventTime, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return calendarapi.EventTime{}, fmt.Errorf("日時が空です")
	}
	if strings.Contains(trimmed, "T") {
		if _, err := time.Parse(time.RFC3339, trimmed); err != nil {
			return calendarapi.EventTime{}, fmt.Errorf("RFC3339形式で指定してください: %s", trimmed)
		}
		return calendarapi.EventTime{DateTime: trimmed}, nil
	}
	if _, err := time.Parse("2006-01-02", trimmed); err != nil {
		return calendarapi.EventTime{}, fmt.Errorf("YYYY-MM-DD形式で指定してください: %s", trimmed)
	}
	return calendarapi.EventTime{Date: trimmed}, nil
}

func parseTimeOrDate(raw string) (time.Time, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, fmt.Errorf("empty")
	}
	if strings.Contains(trimmed, "T") {
		return time.Parse(time.RFC3339, trimmed)
	}
	return time.ParseInLocation("2006-01-02", trimmed, defaultLocation)
}

func asString(v any) string {
	if v == nil {
		return ""
	}
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
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(val))
		if err != nil {
			return 0, err
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("unsupported type %T", v)
	}
}

var _ tools.Handler = (*listTool)(nil)
var _ tools.DefinitionProvider = (*listTool)(nil)
var _ tools.Handler = (*createTool)(nil)
var _ tools.DefinitionProvider = (*createTool)(nil)
var _ tools.Handler = (*updateTool)(nil)
var _ tools.DefinitionProvider = (*updateTool)(nil)
