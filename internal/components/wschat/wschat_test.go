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
