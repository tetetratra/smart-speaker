package router

import (
	"context"
	"testing"
	"time"

	types "smart-speaker/internal/types"
)

func TestStageRoutesSpeechToAudioAndCommit(t *testing.T) {
	st := NewStage(Config{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st.Run(ctx)
	defer st.Close()

	st.Upstream <- types.Event{Kind: types.EventScheduledItem, Payload: types.PlayableSpeech{GenerationID: 1, Text: "はい", Audio: "abc"}}

	expectKind(t, st.Downstream, types.EventRealtimeAudio)
	expectKind(t, st.Downstream, types.EventConversationCommitRequest)
}

func TestStageRoutesToolRequest(t *testing.T) {
	st := NewStage(Config{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st.Run(ctx)
	defer st.Close()

	st.Upstream <- types.Event{Kind: types.EventScheduledItem, Payload: types.ToolRequest{Name: "get_temp", GenerationID: 1}}
	evt := expectKind(t, st.Downstream, types.EventToolRequest)
	req := evt.Payload.(types.ToolRequest)
	if req.Name != "get_temp" {
		t.Fatalf("Name = %q", req.Name)
	}
}

func expectKind(t *testing.T, ch <-chan types.Event, kind types.EventKind) types.Event {
	t.Helper()
	select {
	case evt := <-ch:
		if evt.Kind != kind {
			t.Fatalf("Kind = %s, want %s", evt.Kind, kind)
		}
		return evt
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting %s", kind)
		return types.Event{}
	}
}
