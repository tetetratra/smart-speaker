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
	current := types.EventTimelineItem{TimelineItem: types.TimelineItem{GenerationID: 2}}
	stale := types.EventTimelineItem{TimelineItem: types.TimelineItem{GenerationID: 1}}
	noGeneration := types.EventRealtimeStatus{RealtimeStatus: types.RealtimeStatus{Status: "connecting"}}
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
