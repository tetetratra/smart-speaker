package conversation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	calendarapi "smart-speaker/internal/googlecalendar"
	toolcalendar "smart-speaker/internal/tools/functions/googlecalendar"
)

func TestSharedCalendarClientAcrossConversationAndTools(t *testing.T) {
	loc := time.FixedZone("JST", 9*60*60)
	day0 := time.Date(2026, 3, 21, 0, 0, 0, 0, loc)
	dayN := day0.AddDate(0, 0, calendarPromptDays)
	listCalls := 0
	createCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			listCalls++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{{
					"id":      "evt-1",
					"summary": "朝会",
					"start":   map[string]any{"dateTime": day0.Add(9 * time.Hour).Format(time.RFC3339)},
					"end":     map[string]any{"dateTime": day0.Add(10 * time.Hour).Format(time.RFC3339)},
				}},
			})
		case http.MethodPost:
			createCalls++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "evt-created",
				"summary": "追加予定",
				"start":   map[string]any{"dateTime": day0.Add(13 * time.Hour).Format(time.RFC3339)},
				"end":     map[string]any{"dateTime": day0.Add(14 * time.Hour).Format(time.RFC3339)},
			})
		default:
			t.Fatalf("unexpected method: %s", r.Method)
		}
	}))
	defer server.Close()

	client := calendarapi.NewClient(calendarapi.Config{
		BaseURL:      server.URL,
		HTTPClient:   server.Client(),
		AccessToken:  func(context.Context) (string, error) { return "test-token", nil },
		ListCacheTTL: 5 * time.Minute,
	})

	prompt1, err := buildCalendarContextWithClient(context.Background(), client, day0, dayN)
	if err != nil {
		t.Fatalf("buildCalendarContextWithClient() error = %v", err)
	}
	if !strings.Contains(prompt1, "朝会") || !strings.Contains(prompt1, "[今日]") {
		t.Fatalf("prompt1 = %q", prompt1)
	}
	if listCalls != 1 {
		t.Fatalf("listCalls after first prompt = %d, want 1", listCalls)
	}

	listTool := toolcalendar.NewList(client)
	out, err := listTool.Run(map[string]any{
		"calendar_id": "primary",
		"time_min":    day0.Format(time.RFC3339),
		"time_max":    dayN.Format(time.RFC3339),
		"max_results": 30,
	})
	if err != nil {
		t.Fatalf("listTool.Run() error = %v", err)
	}
	items, _ := out["items"].([]map[string]any)
	if len(items) != 1 {
		t.Fatalf("items len = %d, want 1", len(items))
	}
	if listCalls != 1 {
		t.Fatalf("listCalls after shared cache = %d, want 1", listCalls)
	}

	createTool := toolcalendar.NewCreate(client)
	_, err = createTool.Run(map[string]any{
		"summary":    "追加予定",
		"start_time": day0.Add(13 * time.Hour).Format(time.RFC3339),
		"end_time":   day0.Add(14 * time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("createTool.Run() error = %v", err)
	}
	if createCalls != 1 {
		t.Fatalf("createCalls = %d, want 1", createCalls)
	}

	_, err = buildCalendarContextWithClient(context.Background(), client, day0, dayN)
	if err != nil {
		t.Fatalf("buildCalendarContextWithClient() second error = %v", err)
	}
	if listCalls != 2 {
		t.Fatalf("listCalls after invalidation = %d, want 2", listCalls)
	}
}
