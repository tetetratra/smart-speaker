package googlecalendar

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"smart-speaker/internal/oauth/googlecalendar"
	"smart-speaker/internal/tools"
)

const (
	listToolName   = "google_calendar_list"
	createToolName = "google_calendar_create"
	updateToolName = "google_calendar_update"
)

type baseTool struct {
	client *http.Client
	ctx    context.Context
}

func newBaseTool() *baseTool {
	return &baseTool{client: &http.Client{}}
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

func (t *baseTool) accessToken(ctx context.Context) (string, error) {
	return googlecalendar.AccessToken(ctx)
}

func (t *baseTool) doRequest(ctx context.Context, method, url string, body any) ([]byte, int, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		reader = strings.NewReader(string(raw))
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return nil, 0, err
	}
	token, err := t.accessToken(ctx)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(resp.Body)
	return payload, resp.StatusCode, nil
}

type listTool struct {
	*baseTool
}

func NewList() *listTool {
	return &listTool{baseTool: newBaseTool()}
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

	var timeMin, timeMax string
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
		timeMin = minParsed.Format(time.RFC3339)
		timeMax = maxParsed.Format(time.RFC3339)
	} else if dateRaw != "" {
		start, err := time.ParseInLocation("2006-01-02", dateRaw, time.Local)
		if err != nil {
			return nil, fmt.Errorf("date はYYYY-MM-DDで指定してください")
		}
		timeMin = start.Format(time.RFC3339)
		timeMax = start.Add(24 * time.Hour).Format(time.RFC3339)
	} else {
		return nil, fmt.Errorf("date もしくは time_min/time_max を指定してください")
	}

	query := url.Values{}
	query.Set("timeMin", timeMin)
	query.Set("timeMax", timeMax)
	query.Set("singleEvents", "true")
	query.Set("orderBy", "startTime")
	query.Set("maxResults", fmt.Sprintf("%d", maxResults))

	endpoint := fmt.Sprintf("https://www.googleapis.com/calendar/v3/calendars/%s/events?%s", url.PathEscape(calendarID), query.Encode())
	ctx := t.ctxOrBackground()
	body, status, err := t.doRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if status >= 300 {
		return nil, fmt.Errorf("google calendar list error: %s", strings.TrimSpace(string(body)))
	}

	var resp eventsListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(resp.Items))
	for _, item := range resp.Items {
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
		"time_min":    timeMin,
		"time_max":    timeMax,
		"items":       items,
	}, nil
}

func (t *listTool) Definition() map[string]any {
	return map[string]any{
		"type":        "function",
		"name":        listToolName,
		"description": "Googleカレンダーの予定一覧を取得します。date か time_min/time_max を指定してください。",
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

func NewCreate() *createTool {
	return &createTool{baseTool: newBaseTool()}
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

	payload := map[string]any{
		"summary": summary,
		"start":   startTime,
		"end":     endTime,
	}
	if desc := strings.TrimSpace(asString(args["description"])); desc != "" {
		payload["description"] = desc
	}
	if loc := strings.TrimSpace(asString(args["location"])); loc != "" {
		payload["location"] = loc
	}

	endpoint := fmt.Sprintf("https://www.googleapis.com/calendar/v3/calendars/%s/events", url.PathEscape(calendarID))
	ctx := t.ctxOrBackground()
	body, status, err := t.doRequest(ctx, http.MethodPost, endpoint, payload)
	if err != nil {
		return nil, err
	}
	if status >= 300 {
		return nil, fmt.Errorf("google calendar create error: %s", strings.TrimSpace(string(body)))
	}
	var event eventResponse
	if err := json.Unmarshal(body, &event); err != nil {
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

func NewUpdate() *updateTool {
	return &updateTool{baseTool: newBaseTool()}
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

	endpoint := fmt.Sprintf("https://www.googleapis.com/calendar/v3/calendars/%s/events/%s", url.PathEscape(calendarID), url.PathEscape(eventID))
	ctx := t.ctxOrBackground()

	if action == "delete" {
		body, status, err := t.doRequest(ctx, http.MethodDelete, endpoint, nil)
		if err != nil {
			return nil, err
		}
		if status >= 300 {
			return nil, fmt.Errorf("google calendar delete error: %s", strings.TrimSpace(string(body)))
		}
		return map[string]any{
			"deleted":     true,
			"event_id":    eventID,
			"calendar_id": calendarID,
		}, nil
	}

	payload := map[string]any{}
	if summary := strings.TrimSpace(asString(args["summary"])); summary != "" {
		payload["summary"] = summary
	}
	if desc := strings.TrimSpace(asString(args["description"])); desc != "" {
		payload["description"] = desc
	}
	if loc := strings.TrimSpace(asString(args["location"])); loc != "" {
		payload["location"] = loc
	}
	if startRaw := strings.TrimSpace(asString(args["start_time"])); startRaw != "" {
		startTime, err := buildEventTime(startRaw)
		if err != nil {
			return nil, err
		}
		payload["start"] = startTime
	}
	if endRaw := strings.TrimSpace(asString(args["end_time"])); endRaw != "" {
		endTime, err := buildEventTime(endRaw)
		if err != nil {
			return nil, err
		}
		payload["end"] = endTime
	}
	if len(payload) == 0 {
		return nil, fmt.Errorf("update の場合は更新内容を指定してください")
	}

	body, status, err := t.doRequest(ctx, http.MethodPatch, endpoint, payload)
	if err != nil {
		return nil, err
	}
	if status >= 300 {
		return nil, fmt.Errorf("google calendar update error: %s", strings.TrimSpace(string(body)))
	}
	var event eventResponse
	if err := json.Unmarshal(body, &event); err != nil {
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

type eventsListResponse struct {
	Items []eventResponse `json:"items"`
}

type eventResponse struct {
	ID          string         `json:"id"`
	Summary     string         `json:"summary"`
	Description string         `json:"description"`
	Location    string         `json:"location"`
	HTMLLink    string         `json:"htmlLink"`
	Start       eventDateTime  `json:"start"`
	End         eventDateTime  `json:"end"`
}

type eventDateTime struct {
	Date     string `json:"date"`
	DateTime string `json:"dateTime"`
	TimeZone string `json:"timeZone"`
}

func renderEventTime(t eventDateTime) map[string]any {
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

func buildEventTime(raw string) (map[string]any, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("日時が空です")
	}
	if strings.Contains(trimmed, "T") {
		if _, err := time.Parse(time.RFC3339, trimmed); err != nil {
			return nil, fmt.Errorf("RFC3339形式で指定してください: %s", trimmed)
		}
		return map[string]any{"dateTime": trimmed}, nil
	}
	if _, err := time.Parse("2006-01-02", trimmed); err != nil {
		return nil, fmt.Errorf("YYYY-MM-DD形式で指定してください: %s", trimmed)
	}
	return map[string]any{"date": trimmed}, nil
}

func parseTimeOrDate(raw string) (time.Time, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, fmt.Errorf("empty")
	}
	if strings.Contains(trimmed, "T") {
		return time.Parse(time.RFC3339, trimmed)
	}
	return time.ParseInLocation("2006-01-02", trimmed, time.Local)
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
	default:
		return 0, fmt.Errorf("not an integer")
	}
}

var _ tools.Handler = (*listTool)(nil)
var _ tools.Handler = (*createTool)(nil)
var _ tools.Handler = (*updateTool)(nil)
var _ tools.ContextAware = (*listTool)(nil)
var _ tools.ContextAware = (*createTool)(nil)
var _ tools.ContextAware = (*updateTool)(nil)
var _ tools.DefinitionProvider = (*listTool)(nil)
var _ tools.DefinitionProvider = (*createTool)(nil)
var _ tools.DefinitionProvider = (*updateTool)(nil)
