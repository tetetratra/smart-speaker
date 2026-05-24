package router

import (
	"context"
	"encoding/json"
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

	st.Upstream <- types.Event{Kind: types.EventScheduledItem, Payload: types.ToolRequest{
		ToolCallID:   "call-1",
		Name:         "get_temp",
		Arguments:    json.RawMessage(`{"room":"living"}`),
		GenerationID: 1,
	}}
	commitEvt := expectKind(t, st.Downstream, types.EventConversationCommitRequest)
	commit := commitEvt.Payload.(types.ConversationCommitRequest)
	if commit.ToolCall == nil {
		t.Fatalf("ToolCall is nil")
	}
	if commit.ToolCall.Name != "get_temp" {
		t.Fatalf("ToolCall.Name = %q, want get_temp", commit.ToolCall.Name)
	}
	if string(commit.ToolCall.Arguments) != `{"room":"living"}` {
		t.Fatalf("ToolCall.Arguments = %s, want room args", commit.ToolCall.Arguments)
	}

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
