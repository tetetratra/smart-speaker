package generationfilter

import (
	"context"
	"testing"

	"github.com/tetetratra/smart-speaker/internal/graph"
	"github.com/tetetratra/smart-speaker/internal/states/generation"
	"github.com/tetetratra/smart-speaker/internal/types"
)

func TestAttachPassesOnlyCurrentGeneration(t *testing.T) {
	store := generation.NewStore()
	store.Next() // 1
	store.Next() // 2
	st := &graph.Stage{}
	Attach(st, store)

	if !st.LatestOnly {
		t.Fatal("LatestOnly = false, want true")
	}
	ctx := context.Background()
	current := types.Event{Kind: types.EventTimelineItem, Payload: types.TimelineItem{GenerationID: 2}}
	stale := types.Event{Kind: types.EventTimelineItem, Payload: types.TimelineItem{GenerationID: 1}}
	noGeneration := types.Event{Kind: types.EventSessionReset, Payload: types.SessionResetEvent{}}
	if !st.ReceiveFilter(ctx, current) {
		t.Fatal("現在の世代の event が捨てられました")
	}
	if st.ReceiveFilter(ctx, stale) {
		t.Fatal("古い世代の event が通りました")
	}
	if !st.ReceiveFilter(ctx, noGeneration) {
		t.Fatal("世代を持たない event が捨てられました")
	}
}

func TestAttachToolRequestsReturnsInterruptedResultForStaleRequest(t *testing.T) {
	store := generation.NewStore()
	store.Next() // 1
	store.Next() // 2
	st := &graph.Stage{Downstream: make(chan types.Event, 1)}
	AttachToolRequests(st, store)

	if st.LatestOnly {
		t.Fatal("LatestOnly = true, want false")
	}
	ctx := context.Background()
	if !st.ReceiveFilter(ctx, types.Event{Kind: types.EventToolRequest, Payload: types.ToolRequest{ToolCallID: "now", GenerationID: 2}}) {
		t.Fatal("現在の世代のツール要求が捨てられました")
	}
	if st.ReceiveFilter(ctx, types.Event{Kind: types.EventToolRequest, Payload: types.ToolRequest{ToolCallID: "old", Name: "web_search", GenerationID: 1}}) {
		t.Fatal("古い世代のツール要求が実行に回りました")
	}
	got := <-st.Downstream
	req, ok := got.Payload.(types.ConversationCommitRequest)
	if got.Kind != types.EventConversationCommitRequest || !ok || req.ToolResult == nil || req.ToolResult.ToolCallID != "old" {
		t.Fatalf("中断結果が返っていません: %+v", got)
	}
	if string(req.ToolResult.Output) != `{"error":"interrupted before execution","executed":false}` {
		t.Fatalf("Output = %s", req.ToolResult.Output)
	}
}
