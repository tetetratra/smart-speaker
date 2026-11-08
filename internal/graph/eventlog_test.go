package graph

import (
	"testing"

	types "github.com/tetetratra/smart-speaker/internal/types"
)

func TestShouldLogForwardEvent(t *testing.T) {
	g := New()

	if g.shouldLogForwardEvent(types.EventAudioFrame) {
		t.Fatalf("EventAudioFrame should be suppressed")
	}
	if g.shouldLogForwardEvent(types.EventRTCPeerAudioFrame) {
		t.Fatalf("EventRTCPeerAudioFrame should be suppressed")
	}
	if !g.shouldLogForwardEvent(types.EventRealtimeOutput) {
		t.Fatalf("EventRealtimeOutput should still be logged")
	}
}
