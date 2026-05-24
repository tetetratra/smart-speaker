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

	audioEvt := expectKind(t, st.Downstream, types.EventRealtimeAudio)
	audio := audioEvt.Payload.(types.OutputAudio)
	if audio.Role != types.RoleAgent {
		t.Fatalf("audio role = %s, want agent", audio.Role)
	}
	commitEvt := expectKind(t, st.Downstream, types.EventConversationCommitRequest)
	req := commitEvt.Payload.(types.ConversationCommitRequest)
	if req.Role != types.RoleAgent {
		t.Fatalf("commit role = %s, want agent", req.Role)
	}
}

func TestStageRoutesToolRequest(t *testing.T) {
	st := NewStage(Config{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st.Run(ctx)
	defer st.Close()

	st.Upstream <- types.Event{Kind: types.EventScheduledItem, Payload: types.ToolRequest{ToolCallID: "call-1", Name: "get_temp", Arguments: []byte(`{"place":"living"}`), GenerationID: 1}}
	commitEvt := expectKind(t, st.Downstream, types.EventConversationCommitRequest)
	commit := commitEvt.Payload.(types.ConversationCommitRequest)
	if commit.Role != types.RoleToolCall || commit.ToolCall == nil {
		t.Fatalf("commit = %#v, want tool_call payload", commit)
	}
	if commit.ToolCall.ToolCallID != "call-1" || string(commit.ToolCall.Arguments) != `{"place":"living"}` {
		t.Fatalf("tool call = %#v", commit.ToolCall)
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
