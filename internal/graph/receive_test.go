package graph_test

import (
	"context"
	"github.com/tetetratra/smart-speaker/internal/components/generationfilter"
	"github.com/tetetratra/smart-speaker/internal/graph"
	"github.com/tetetratra/smart-speaker/internal/states/generation"
	"github.com/tetetratra/smart-speaker/internal/types"
	"testing"
	"time"
)

func TestFilterChecksAfterBlockedHandoff(t *testing.T) {
	st := &graph.Stage{Upstream: make(chan types.Event)}
	store := generation.NewStore()
	store.Next()
	generationfilter.Attach(st, store)
	old := types.Event{Payload: types.TimelineItem{GenerationID: 1}}
	started := make(chan struct{})
	go func() {
		close(started)
		st.Upstream <- old
		st.Upstream <- types.Event{Payload: types.TimelineItem{GenerationID: 2}}
	}()
	<-started
	store.Next()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	e, ok := st.Receive(ctx)
	if !ok || e.Payload.(types.TimelineItem).GenerationID != 2 {
		t.Fatal(e, ok)
	}
}
