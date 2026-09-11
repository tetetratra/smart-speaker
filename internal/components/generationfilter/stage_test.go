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

func TestStageHoldsPausedGenerationAndFlushesOnResume(t *testing.T) {
	store := generation.NewStore()
	store.Next()
	candidateID := store.BeginInterruption()
	st := NewStage(Config{Generation: store})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st.Run(ctx)
	defer st.Close()

	st.Upstream <- types.Event{Kind: types.EventTimelineItem, Payload: types.TimelineItem{GenerationID: 1, Kind: types.TimelineKindSpeech, Text: "paused"}}
	assertNoEvent(t, st.Downstream)

	if !store.ResumeIfPending(candidateID) {
		t.Fatal("ResumeIfPending returned false")
	}
	select {
	case evt := <-st.Downstream:
		item := evt.Payload.(types.TimelineItem)
		if item.Text != "paused" {
			t.Fatalf("Text = %q, want paused", item.Text)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting held item")
	}
}

func TestStageDropsHeldPausedGenerationOnConfirm(t *testing.T) {
	store := generation.NewStore()
	store.Next()
	candidateID := store.BeginInterruption()
	st := NewStage(Config{Generation: store})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st.Run(ctx)
	defer st.Close()

	st.Upstream <- types.Event{Kind: types.EventTimelineItem, Payload: types.TimelineItem{GenerationID: 1, Kind: types.TimelineKindSpeech, Text: "paused"}}
	assertNoEvent(t, st.Downstream)

	if !store.ConfirmIfPending(candidateID) {
		t.Fatal("ConfirmIfPending returned false")
	}
	assertNoEvent(t, st.Downstream)

	st.Upstream <- types.Event{Kind: types.EventTimelineItem, Payload: types.TimelineItem{GenerationID: 2, Kind: types.TimelineKindSpeech, Text: "candidate"}}
	select {
	case evt := <-st.Downstream:
		item := evt.Payload.(types.TimelineItem)
		if item.Text != "candidate" {
			t.Fatalf("Text = %q, want candidate", item.Text)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting candidate item")
	}
}

func TestStagePassesCandidateGenerationWhilePending(t *testing.T) {
	store := generation.NewStore()
	store.Next()
	candidateID := store.BeginInterruption()
	st := NewStage(Config{Generation: store})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st.Run(ctx)
	defer st.Close()

	st.Upstream <- types.Event{Kind: types.EventTimelineItem, Payload: types.TimelineItem{GenerationID: candidateID, Kind: types.TimelineKindSpeech, Text: "candidate"}}
	select {
	case evt := <-st.Downstream:
		item := evt.Payload.(types.TimelineItem)
		if item.Text != "candidate" {
			t.Fatalf("Text = %q, want candidate", item.Text)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting candidate item")
	}
}

func TestStagePassesAllEventsWhenGenerationStoreIsNil(t *testing.T) {
	st := NewStage(Config{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st.Run(ctx)
	defer st.Close()

	st.Upstream <- types.Event{Kind: types.EventSpeechStart, Payload: types.SpeechEvent{}}
	select {
	case evt := <-st.Downstream:
		if evt.Kind != types.EventSpeechStart {
			t.Fatalf("Kind = %s, want EventSpeechStart", evt.Kind)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting event")
	}
}

func assertNoEvent(t *testing.T, ch <-chan types.Event) {
	t.Helper()
	select {
	case evt := <-ch:
		t.Fatalf("unexpected event: %#v", evt)
	case <-time.After(50 * time.Millisecond):
	}
}
