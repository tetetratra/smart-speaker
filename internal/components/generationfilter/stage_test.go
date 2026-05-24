package generationfilter

import (
	"context"
	"testing"
	"time"

	"github.com/tetetratra/smart-speaker/internal/states/generation"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

func TestStagePassesOnlyCurrentGeneration(t *testing.T) {
	store := generation.NewStore()
	store.Next()
	store.Next()
	st := NewStage(Config{Generation: store})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st.Run(ctx)
	defer st.Close()

	st.Upstream <- types.Event{Kind: types.EventTimelineItem, Payload: types.TimelineItem{GenerationID: 1, Kind: types.TimelineKindSpeech, Text: "old"}}
	st.Upstream <- types.Event{Kind: types.EventTimelineItem, Payload: types.TimelineItem{GenerationID: 2, Kind: types.TimelineKindSpeech, Text: "current"}}

	select {
	case evt := <-st.Downstream:
		item := evt.Payload.(types.TimelineItem)
		if item.Text != "current" {
			t.Fatalf("Text = %q, want current", item.Text)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting current item")
	}
}
