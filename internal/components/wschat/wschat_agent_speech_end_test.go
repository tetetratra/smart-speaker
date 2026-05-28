package wschat

import (
	"testing"
	"time"

	types "github.com/tetetratra/smart-speaker/internal/types"
)

func TestMessageForAgentSpeechPlaybackEnd(t *testing.T) {
	completedAt := time.Date(2026, 5, 28, 16, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	msg, targetID, ok := messageForEvent(types.Event{
		Kind: types.EventAgentSpeechPlaybackEnd,
		Payload: types.AgentSpeechPlaybackEnd{
			GenerationID: 3,
			CompletedAt:  completedAt,
		},
	})
	if !ok {
		t.Fatal("messageForEvent returned ok=false")
	}
	if targetID != "" {
		t.Fatalf("targetID = %q, want empty", targetID)
	}
	if got := msg["type"]; got != "agent_speech_end" {
		t.Fatalf("type = %v, want agent_speech_end", got)
	}
	if got := msg["generation_id"]; got != uint64(3) {
		t.Fatalf("generation_id = %v, want 3", got)
	}
	if got := msg["completed_at"]; got != completedAt.Format(time.RFC3339Nano) {
		t.Fatalf("completed_at = %v, want %s", got, completedAt.Format(time.RFC3339Nano))
	}
}
