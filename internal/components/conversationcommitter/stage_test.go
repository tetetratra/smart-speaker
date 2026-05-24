package conversationcommitter

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"smart-speaker/internal/states/conversationhistory"
	"smart-speaker/internal/states/generation"
	types "smart-speaker/internal/types"
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

	expectEvent(t, st.Downstream, types.EventLLMRequest)
	records := history.Snapshot()
	if len(records) != 1 {
		t.Fatalf("history len = %d, want 1", len(records))
	}
	if records[0].Metadata["stale"] != true {
		t.Fatalf("stale = %v, want true", records[0].Metadata["stale"])
	}
}

func TestStageCommitsToolCallWithoutOutputEvent(t *testing.T) {
	history := conversationhistory.NewStore()
	gen := generation.NewStore()
	st, _ := NewStage(Config{History: history, Generation: gen})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st.Run(ctx)
	defer st.Close()

	st.Upstream <- types.Event{Kind: types.EventConversationCommitRequest, Payload: types.ConversationCommitRequest{
		Role:         types.RoleToolCall,
		GenerationID: 1,
		ToolCall: &types.ToolCallRecord{
			ToolCallID:   "call-1",
			Name:         "web_search",
			Arguments:    json.RawMessage(`{"query":"東京 天気"}`),
			GenerationID: 1,
		},
	}}

	select {
	case evt := <-st.Downstream:
		t.Fatalf("unexpected event: %s", evt.Kind)
	case <-time.After(50 * time.Millisecond):
	}
	records := history.Snapshot()
	if len(records) != 1 {
		t.Fatalf("history len = %d, want 1", len(records))
	}
	if records[0].Role != types.RoleToolCall {
		t.Fatalf("Role = %s, want tool_call", records[0].Role)
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
