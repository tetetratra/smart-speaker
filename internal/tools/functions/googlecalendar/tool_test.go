package googlecalendar

import (
	"context"
	"testing"
	"time"

	calendarapi "smart-speaker/internal/googlecalendar"
)

type stubClient struct {
	listReq    *calendarapi.ListEventsRequest
	createReq  *calendarapi.CreateEventRequest
	updateReq  *calendarapi.UpdateEventRequest
	deleteReq  *calendarapi.DeleteEventRequest
	listResp   []calendarapi.Event
	createResp calendarapi.Event
	updateResp calendarapi.Event
}

func (s *stubClient) ListEvents(_ context.Context, req calendarapi.ListEventsRequest) ([]calendarapi.Event, error) {
	copyReq := req
	s.listReq = &copyReq
	return s.listResp, nil
}

func (s *stubClient) CreateEvent(_ context.Context, req calendarapi.CreateEventRequest) (calendarapi.Event, error) {
	copyReq := req
	s.createReq = &copyReq
	return s.createResp, nil
}

func (s *stubClient) UpdateEvent(_ context.Context, req calendarapi.UpdateEventRequest) (calendarapi.Event, error) {
	copyReq := req
	s.updateReq = &copyReq
	return s.updateResp, nil
}

func (s *stubClient) DeleteEvent(_ context.Context, req calendarapi.DeleteEventRequest) error {
	copyReq := req
	s.deleteReq = &copyReq
	return nil
}

func TestListToolRunWithDate(t *testing.T) {
	client := &stubClient{listResp: []calendarapi.Event{{ID: "evt-1", Summary: "朝会"}}}
	tool := NewList(client)

	out, err := tool.Run(map[string]any{
		"date":        "2026-03-21",
		"max_results": 10,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if client.listReq == nil {
		t.Fatal("listReq is nil")
	}
	if client.listReq.CalendarID != "primary" {
		t.Fatalf("calendar_id = %q", client.listReq.CalendarID)
	}
	if client.listReq.TimeMin.Format(time.RFC3339) != "2026-03-21T00:00:00+09:00" {
		t.Fatalf("timeMin = %s", client.listReq.TimeMin.Format(time.RFC3339))
	}
	if client.listReq.TimeMax.Sub(client.listReq.TimeMin) != 24*time.Hour {
		t.Fatalf("range = %s", client.listReq.TimeMax.Sub(client.listReq.TimeMin))
	}
	items, ok := out["items"].([]map[string]any)
	if !ok || len(items) != 1 {
		t.Fatalf("items = %#v", out["items"])
	}
}

func TestCreateToolRun(t *testing.T) {
	client := &stubClient{createResp: calendarapi.Event{ID: "evt-2", Summary: "病院", HTMLLink: "https://example.com"}}
	tool := NewCreate(client)

	out, err := tool.Run(map[string]any{
		"summary":     "病院",
		"description": "定期検診",
		"location":    "渋谷",
		"start_time":  "2026-03-21T10:00:00+09:00",
		"end_time":    "2026-03-21T11:00:00+09:00",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if client.createReq == nil {
		t.Fatal("createReq is nil")
	}
	if client.createReq.Summary != "病院" || client.createReq.Description != "定期検診" || client.createReq.Location != "渋谷" {
		t.Fatalf("createReq = %#v", client.createReq)
	}
	if client.createReq.Start.DateTime != "2026-03-21T10:00:00+09:00" || client.createReq.End.DateTime != "2026-03-21T11:00:00+09:00" {
		t.Fatalf("start/end = %#v %#v", client.createReq.Start, client.createReq.End)
	}
	if out["id"] != "evt-2" {
		t.Fatalf("out = %#v", out)
	}
}

func TestUpdateToolRunUpdate(t *testing.T) {
	client := &stubClient{updateResp: calendarapi.Event{ID: "evt-3", Summary: "更新後"}}
	tool := NewUpdate(client)

	out, err := tool.Run(map[string]any{
		"event_id":    "evt-3",
		"summary":     "更新後",
		"description": "説明",
		"start_time":  "2026-03-21",
		"end_time":    "2026-03-22",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if client.updateReq == nil {
		t.Fatal("updateReq is nil")
	}
	if client.updateReq.EventID != "evt-3" {
		t.Fatalf("eventID = %q", client.updateReq.EventID)
	}
	if client.updateReq.Summary == nil || *client.updateReq.Summary != "更新後" {
		t.Fatalf("summary = %#v", client.updateReq.Summary)
	}
	if client.updateReq.Start == nil || client.updateReq.Start.Date != "2026-03-21" {
		t.Fatalf("start = %#v", client.updateReq.Start)
	}
	if out["summary"] != "更新後" {
		t.Fatalf("out = %#v", out)
	}
}

func TestUpdateToolRunDelete(t *testing.T) {
	client := &stubClient{}
	tool := NewUpdate(client)

	out, err := tool.Run(map[string]any{
		"action":   "delete",
		"event_id": "evt-4",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if client.deleteReq == nil {
		t.Fatal("deleteReq is nil")
	}
	if client.deleteReq.EventID != "evt-4" || client.deleteReq.CalendarID != "primary" {
		t.Fatalf("deleteReq = %#v", client.deleteReq)
	}
	if out["deleted"] != true {
		t.Fatalf("out = %#v", out)
	}
}
