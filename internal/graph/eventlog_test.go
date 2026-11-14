package graph

import (
	"reflect"
	"testing"

	types "github.com/tetetratra/smart-speaker/internal/types"
)

func TestShouldLogForwardEvent(t *testing.T) {
	g := New()

	if g.shouldLogForwardEvent(reflect.TypeOf(types.EventAudioFrame{})) {
		t.Fatalf("EventAudioFrame should be suppressed")
	}
	if g.shouldLogForwardEvent(reflect.TypeOf(types.EventRTCPeerAudioFrame{})) {
		t.Fatalf("EventRTCPeerAudioFrame should be suppressed")
	}
	if !g.shouldLogForwardEvent(reflect.TypeOf(types.EventRealtimeOutput{})) {
		t.Fatalf("EventRealtimeOutput should still be logged")
	}
}
