package sessionactivate

import (
	"context"
	"testing"
	"time"

	"github.com/tetetratra/smart-speaker/internal/states/agentstatus"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

func TestStagePassesEventsThrough(t *testing.T) {
	status := agentstatus.NewStore()
	st := NewStage(Config{AgentStatus: status})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st.Run(ctx)
	defer st.Close()

	input := types.Event{
		Kind: types.EventTimelineItem,
		Payload: types.TimelineItem{
			Kind: types.TimelineKindWait,
			Sec:  1,
		},
	}
	st.Upstream <- input

	select {
	case got := <-st.Downstream:
		if got.Kind != input.Kind {
			t.Fatalf("kind = %s, want %s", got.Kind, input.Kind)
		}
		item, ok := got.Payload.(types.TimelineItem)
		if !ok {
			t.Fatalf("payload type = %T, want types.TimelineItem", got.Payload)
		}
		if item.Kind != types.TimelineKindWait {
			t.Fatalf("item kind = %q, want %q", item.Kind, types.TimelineKindWait)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting downstream event")
	}
}

func TestStageSetsActiveWhenSpeechPasses(t *testing.T) {
	status := agentstatus.NewStore()
	st := NewStage(Config{AgentStatus: status})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st.Run(ctx)
	defer st.Close()

	st.Upstream <- types.Event{
		Kind: types.EventTimelineItem,
		Payload: types.TimelineItem{
			Kind: types.TimelineKindSpeech,
			Text: "こんにちは",
		},
	}

	select {
	case <-st.Downstream:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting downstream event")
	}
	if got := status.Status(); got != agentstatus.StatusActive {
		t.Fatalf("status = %q, want %q", got, agentstatus.StatusActive)
	}
}

func TestStageDoesNotSetActiveForNonSpeech(t *testing.T) {
	status := agentstatus.NewStore()
	st := NewStage(Config{AgentStatus: status})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st.Run(ctx)
	defer st.Close()

	st.Upstream <- types.Event{
		Kind: types.EventTimelineItem,
		Payload: types.TimelineItem{
			Kind: types.TimelineKindTool,
		},
	}

	select {
	case <-st.Downstream:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting downstream event")
	}
	if got := status.Status(); got != agentstatus.StatusIdle {
		t.Fatalf("status = %q, want %q", got, agentstatus.StatusIdle)
	}
}
