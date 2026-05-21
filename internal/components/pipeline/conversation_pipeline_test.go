package pipeline

import (
	"context"
	"testing"
	"time"

	"smart-speaker/internal/components/generationfilter"
	"smart-speaker/internal/components/router"
	"smart-speaker/internal/components/scheduler"
	"smart-speaker/internal/states/generation"
	types "smart-speaker/internal/types"
)

func TestSchedulerRouterKeepsSpeechBeforeTool(t *testing.T) {
	store := generation.NewStore()
	store.Next()
	sched := scheduler.NewStage(scheduler.Config{})
	filter := generationfilter.NewStage(generationfilter.Config{Generation: store})
	rt := router.NewStage(router.Config{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sched.Run(ctx)
	filter.Run(ctx)
	rt.Run(ctx)
	defer sched.Close()
	defer filter.Close()
	defer rt.Close()

	go pump(ctx, sched.Downstream, filter.Upstream)
	go pump(ctx, filter.Downstream, rt.Upstream)

	sched.Upstream <- types.Event{Kind: types.EventPlayableSpeech, Payload: types.PlayableSpeech{GenerationID: 1, Text: "確認するね", Audio: "abc", DurationSeconds: 0.01}}
	sched.Upstream <- types.Event{Kind: types.EventTimelineItem, Payload: types.TimelineItem{Kind: types.TimelineKindTool, GenerationID: 1, SequenceID: "2", ToolName: "get_temp"}}

	if evt := expect(t, rt.Downstream); evt.Kind != types.EventRealtimeAudio {
		t.Fatalf("first Kind = %s, want EventRealtimeAudio", evt.Kind)
	}
	if evt := expect(t, rt.Downstream); evt.Kind != types.EventConversationCommitRequest {
		t.Fatalf("second Kind = %s, want EventConversationCommitRequest", evt.Kind)
	}
	if evt := expect(t, rt.Downstream); evt.Kind != types.EventToolRequest {
		t.Fatalf("third Kind = %s, want EventToolRequest", evt.Kind)
	}
}

func pump(ctx context.Context, in <-chan types.Event, out chan<- types.Event) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-in:
			if !ok {
				return
			}
			select {
			case <-ctx.Done():
				return
			case out <- evt:
			}
		}
	}
}

func expect(t *testing.T, ch <-chan types.Event) types.Event {
	t.Helper()
	select {
	case evt := <-ch:
		return evt
	case <-time.After(time.Second):
		t.Fatal("timeout waiting event")
		return types.Event{}
	}
}
