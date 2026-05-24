package conversationcommitter

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/tetetratra/smart-speaker/internal/states/conversationhistory"
	"github.com/tetetratra/smart-speaker/internal/states/generation"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

func TestStageCommitsUserBeforeLLMRequest(t *testing.T) {
	history := conversationhistory.NewStore()
	gen := generation.NewStore()
	st, _ := NewStage(Config{History: history, Generation: gen})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st.Run(ctx)
	defer st.Close()

	st.Upstream <- types.Event{Kind: types.EventConversationCommitRequest, Payload: types.ConversationCommitRequest{
		Role:         types.RoleUser,
		Text:         "こんにちは",
		GenerationID: 1,
		Source:       "stt",
	}}

	expectEvent(t, st.Downstream, types.EventRealtimeOutput)
	expectEvent(t, st.Downstream, types.EventLLMRequest)
	if got := len(history.Snapshot()); got != 1 {
		t.Fatalf("history len = %d, want 1", got)
	}
}

func TestResultAPICommitsToolResultAsStale(t *testing.T) {
	history := conversationhistory.NewStore()
	gen := generation.NewStore()
	gen.Next()
	gen.Next()
	st, api := NewStage(Config{History: history, Generation: gen})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st.Run(ctx)
	defer st.Close()

	if err := api.CommitToolResult(ctx, types.ToolResultRecord{
		ToolCallID:   "call-1",
		Name:         "get_temp",
		Output:       json.RawMessage(`{"temp":29}`),
		GenerationID: 1,
	}); err != nil {
		t.Fatal(err)
	}

	expectEvent(t, st.Downstream, types.EventRealtimeOutput)
	expectEvent(t, st.Downstream, types.EventLLMRequest)
	records := history.Snapshot()
	if len(records) != 1 {
		t.Fatalf("history len = %d, want 1", len(records))
	}
	if records[0].Metadata["stale"] != true {
		t.Fatalf("stale = %v, want true", records[0].Metadata["stale"])
	}
}

func TestStageCommitsToolCallToRealtimeOutput(t *testing.T) {
	history := conversationhistory.NewStore()
	gen := generation.NewStore()
	st, _ := NewStage(Config{History: history, Generation: gen})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st.Run(ctx)
	defer st.Close()

	st.Upstream <- types.Event{Kind: types.EventConversationCommitRequest, Payload: types.ConversationCommitRequest{
		Role: types.RoleToolCall,
		ToolCall: &types.ToolCallRecord{
			ToolCallID:   "call-1",
			Name:         "get_temp",
			Arguments:    json.RawMessage(`{"place":"living"}`),
			GenerationID: 1,
		},
	}}

	evt := expectEvent(t, st.Downstream, types.EventRealtimeOutput)
	line := evt.Payload.(types.OutputLine)
	if line.Role != types.RoleToolCall {
		t.Fatalf("OutputLine.Role = %s, want tool_call", line.Role)
	}
	records := history.Snapshot()
	if len(records) != 1 || records[0].Role != types.RoleToolCall {
		t.Fatalf("records = %#v, want one tool_call", records)
	}
}

func expectEvent(t *testing.T, ch <-chan types.Event, kind types.EventKind) types.Event {
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
