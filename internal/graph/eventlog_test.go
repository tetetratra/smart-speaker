package graph

import (
	"testing"

	types "github.com/tetetratra/smart-speaker/internal/types"
)

func TestShouldLogForwardEvent(t *testing.T) {
	g := New()

	if g.shouldLogForwardEvent(types.EventRTCVADStatus) {
		t.Fatalf("EventRTCVADStatus should be suppressed")
	}
	if g.shouldLogForwardEvent(types.EventRealtimeAudio) {
		t.Fatalf("EventRealtimeAudio should be suppressed")
	}
	if g.shouldLogForwardEvent(types.EventRTCPeerAudioFrame) {
		t.Fatalf("EventRTCPeerAudioFrame should be suppressed")
	}
	if g.shouldLogForwardEvent(types.EventRTCSpeechAudio) {
		t.Fatalf("EventRTCSpeechAudio should be suppressed")
	}
	if !g.shouldLogForwardEvent(types.EventRealtimeOutput) {
		t.Fatalf("EventRealtimeOutput should still be logged")
	}
}
