package googlecalendar

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientListEventsUsesCache(t *testing.T) {
	now := time.Date(2026, 3, 21, 10, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	listCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		listCalls++
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("Authorization = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{{
				"id":      "evt-1",
				"summary": "朝会",
				"start":   map[string]any{"dateTime": now.Format(time.RFC3339)},
				"end":     map[string]any{"dateTime": now.Add(30 * time.Minute).Format(time.RFC3339)},
			}},
		})
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL:      server.URL,
		HTTPClient:   server.Client(),
		AccessToken:  func(context.Context) (string, error) { return "test-token", nil },
		ListCacheTTL: 5 * time.Minute,
		Now:          func() time.Time { return now },
	})

	req := ListEventsRequest{
		CalendarID: "primary",
		TimeMin:    now,
		TimeMax:    now.Add(24 * time.Hour),
		MaxResults: 30,
	}

	first, err := client.ListEvents(context.Background(), req)
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	if len(first) != 1 || first[0].Summary != "朝会" {
		t.Fatalf("first = %#v", first)
	}
	second, err := client.ListEvents(context.Background(), req)
	if err != nil {
		t.Fatalf("ListEvents() second error = %v", err)
	}
	if len(second) != 1 || second[0].Summary != "朝会" {
		t.Fatalf("second = %#v", second)
	}
	if listCalls != 1 {
		t.Fatalf("listCalls = %d, want 1", listCalls)
	}

	_, err = client.ListEvents(context.Background(), ListEventsRequest{
		CalendarID: "primary",
		TimeMin:    now,
		TimeMax:    now.Add(48 * time.Hour),
		MaxResults: 30,
	})
	if err != nil {
		t.Fatalf("ListEvents() different request error = %v", err)
	}
	if listCalls != 2 {
		t.Fatalf("listCalls after different key = %d, want 2", listCalls)
	}

	now = now.Add(6 * time.Minute)
	_, err = client.ListEvents(context.Background(), req)
	if err != nil {
		t.Fatalf("ListEvents() after ttl error = %v", err)
	}
	if listCalls != 3 {
		t.Fatalf("listCalls after ttl = %d, want 3", listCalls)
	}
}

func TestClientMutationsInvalidateCache(t *testing.T) {
	now := time.Date(2026, 3, 21, 10, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	listCalls := 0
	createCalls := 0
	updateCalls := 0
	deleteCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			listCalls++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{{
					"id":      "evt-1",
					"summary": "朝会",
					"start":   map[string]any{"dateTime": now.Format(time.RFC3339)},
					"end":     map[string]any{"dateTime": now.Add(30 * time.Minute).Format(time.RFC3339)},
				}},
			})
		case http.MethodPost:
			createCalls++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "evt-created",
				"summary": "追加予定",
				"start":   map[string]any{"dateTime": now.Format(time.RFC3339)},
				"end":     map[string]any{"dateTime": now.Add(time.Hour).Format(time.RFC3339)},
			})
		case http.MethodPatch:
			updateCalls++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "evt-updated",
				"summary": "更新予定",
				"start":   map[string]any{"dateTime": now.Format(time.RFC3339)},
				"end":     map[string]any{"dateTime": now.Add(time.Hour).Format(time.RFC3339)},
			})
		case http.MethodDelete:
			deleteCalls++
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected method: %s", r.Method)
		}
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL:      server.URL,
		HTTPClient:   server.Client(),
		AccessToken:  func(context.Context) (string, error) { return "test-token", nil },
		ListCacheTTL: 5 * time.Minute,
		Now:          func() time.Time { return now },
	})
	listReq := ListEventsRequest{
		CalendarID: "primary",
		TimeMin:    now,
		TimeMax:    now.Add(24 * time.Hour),
		MaxResults: 30,
	}

	if _, err := client.ListEvents(context.Background(), listReq); err != nil {
		t.Fatalf("ListEvents() initial error = %v", err)
	}
	if _, err := client.ListEvents(context.Background(), listReq); err != nil {
		t.Fatalf("ListEvents() cached error = %v", err)
	}
	if listCalls != 1 {
		t.Fatalf("listCalls initial = %d, want 1", listCalls)
	}

	if _, err := client.CreateEvent(context.Background(), CreateEventRequest{
		Summary: "追加予定",
		Start:   EventTime{DateTime: now.Format(time.RFC3339)},
		End:     EventTime{DateTime: now.Add(time.Hour).Format(time.RFC3339)},
	}); err != nil {
		t.Fatalf("CreateEvent() error = %v", err)
	}
	if _, err := client.ListEvents(context.Background(), listReq); err != nil {
		t.Fatalf("ListEvents() after create error = %v", err)
	}
	if listCalls != 2 || createCalls != 1 {
		t.Fatalf("after create: listCalls=%d createCalls=%d", listCalls, createCalls)
	}

	summary := "更新予定"
	if _, err := client.UpdateEvent(context.Background(), UpdateEventRequest{
		EventID: "evt-1",
		Summary: &summary,
	}); err != nil {
		t.Fatalf("UpdateEvent() error = %v", err)
	}
	if _, err := client.ListEvents(context.Background(), listReq); err != nil {
		t.Fatalf("ListEvents() after update error = %v", err)
	}
	if listCalls != 3 || updateCalls != 1 {
		t.Fatalf("after update: listCalls=%d updateCalls=%d", listCalls, updateCalls)
	}

	if err := client.DeleteEvent(context.Background(), DeleteEventRequest{EventID: "evt-1"}); err != nil {
		t.Fatalf("DeleteEvent() error = %v", err)
	}
	if _, err := client.ListEvents(context.Background(), listReq); err != nil {
		t.Fatalf("ListEvents() after delete error = %v", err)
	}
	if listCalls != 4 || deleteCalls != 1 {
		t.Fatalf("after delete: listCalls=%d deleteCalls=%d", listCalls, deleteCalls)
	}
}
