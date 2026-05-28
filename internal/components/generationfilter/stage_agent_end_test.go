package generationfilter

import (
	"context"
	"testing"
	"time"

	"github.com/tetetratra/smart-speaker/internal/states/generation"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

func TestStageDropsStaleAgentTimelineEnd(t *testing.T) {
	store := generation.NewStore()
	store.Next()
	store.Next()
	st := NewStage(Config{Generation: store})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st.Run(ctx)
	defer st.Close()

	st.Upstream <- types.Event{
		Kind:    types.EventAgentTimelineEnd,
		Payload: types.AgentTimelineEnd{GenerationID: 1},
	}
	st.Upstream <- types.Event{
		Kind:    types.EventAgentTimelineEnd,
		Payload: types.AgentTimelineEnd{GenerationID: 2},
	}

	select {
	case evt := <-st.Downstream:
		end := evt.Payload.(types.AgentTimelineEnd)
		if end.GenerationID != 2 {
			t.Fatalf("GenerationID = %d, want 2", end.GenerationID)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting current agent timeline end")
	}
}

func TestStageDropsStaleAgentSpeechPlaybackEnd(t *testing.T) {
	store := generation.NewStore()
	store.Next()
	store.Next()
	st := NewStage(Config{Generation: store})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st.Run(ctx)
	defer st.Close()

	st.Upstream <- types.Event{
		Kind: types.EventAgentSpeechPlaybackEnd,
		Payload: types.AgentSpeechPlaybackEnd{
			GenerationID: 1,
			CompletedAt:  time.Now(),
		},
	}
	st.Upstream <- types.Event{
		Kind: types.EventAgentSpeechPlaybackEnd,
		Payload: types.AgentSpeechPlaybackEnd{
			GenerationID: 2,
			CompletedAt:  time.Now(),
		},
	}

	select {
	case evt := <-st.Downstream:
		end := evt.Payload.(types.AgentSpeechPlaybackEnd)
		if end.GenerationID != 2 {
			t.Fatalf("GenerationID = %d, want 2", end.GenerationID)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting current agent speech playback end")
	}
}
