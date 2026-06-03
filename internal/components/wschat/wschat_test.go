package wschat

import (
	"testing"
	"time"

	types "github.com/tetetratra/smart-speaker/internal/types"
)

func TestMessageForEventSessionReset(t *testing.T) {
	requestedAt := time.Date(2026, 5, 27, 12, 0, 0, 123, time.UTC)
	msg, targetID, ok := messageForEvent(types.Event{
		Kind: types.EventSessionReset,
		Payload: types.SessionResetEvent{
			RequestedAt: requestedAt,
		},
	})

	if !ok {
		t.Fatal("messageForEvent returned ok=false")
	}
	if targetID != "" {
		t.Fatalf("targetID = %q, want empty", targetID)
	}
	if got := msg["type"]; got != "session_reset" {
		t.Fatalf("type = %v, want session_reset", got)
	}
	if got := msg["requested_at"]; got != requestedAt.Format(time.RFC3339Nano) {
		t.Fatalf("requested_at = %v, want %s", got, requestedAt.Format(time.RFC3339Nano))
	}
}

func TestMessageForEventTimerState(t *testing.T) {
	at := time.Date(2026, 6, 3, 21, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	createdAt := time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC)
	msg, targetID, ok := messageForEvent(types.Event{
		Kind: types.EventTimerState,
		Payload: types.TimerState{Timers: []types.TimerStateItem{
			{ID: "timer-1", At: at, Action: "エアコンをoffにする", CreatedAt: createdAt},
		}},
	})

	if !ok {
		t.Fatal("messageForEvent returned ok=false")
	}
	if targetID != "" {
		t.Fatalf("targetID = %q, want empty", targetID)
	}
	if got := msg["type"]; got != "timer.state" {
		t.Fatalf("type = %v, want timer.state", got)
	}
	timers, ok := msg["timers"].([]map[string]any)
	if !ok || len(timers) != 1 {
		t.Fatalf("timers = %#v", msg["timers"])
	}
	if timers[0]["id"] != "timer-1" || timers[0]["action"] != "エアコンをoffにする" {
		t.Fatalf("timer payload = %#v", timers[0])
	}
	if timers[0]["at"] != at.Format(time.RFC3339) {
		t.Fatalf("at = %v, want %s", timers[0]["at"], at.Format(time.RFC3339))
	}
}
