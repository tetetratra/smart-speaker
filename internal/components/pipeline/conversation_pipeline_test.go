package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/tetetratra/smart-speaker/internal/components/generationfilter"
	"github.com/tetetratra/smart-speaker/internal/components/interimstopper"
	"github.com/tetetratra/smart-speaker/internal/components/router"
	"github.com/tetetratra/smart-speaker/internal/components/scheduler"
	"github.com/tetetratra/smart-speaker/internal/components/utterancebuffer"
	"github.com/tetetratra/smart-speaker/internal/states/generation"
	types "github.com/tetetratra/smart-speaker/internal/types"
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
	evt := expect(t, rt.Downstream)
	if evt.Kind != types.EventConversationCommitRequest {
		t.Fatalf("third Kind = %s, want EventConversationCommitRequest", evt.Kind)
	}
	req := evt.Payload.(types.ConversationCommitRequest)
	if req.Role != types.RoleToolCall {
		t.Fatalf("third Role = %s, want tool_call", req.Role)
	}
	if evt := expect(t, rt.Downstream); evt.Kind != types.EventToolRequest {
		t.Fatalf("fourth Kind = %s, want EventToolRequest", evt.Kind)
	}
}

func TestInterimStopsOldGenerationAndFinalCommitsUserUtterance(t *testing.T) {
	store := generation.NewStore()
	store.Next()
	stopper := interimstopper.NewStage(interimstopper.Config{Generation: store})
	utterance := utterancebuffer.NewStage(utterancebuffer.Config{
		Generation: store,
		Delay:      time.Millisecond,
	})
	filter := generationfilter.NewStage(generationfilter.Config{Generation: store})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopper.Run(ctx)
	utterance.Run(ctx)
	filter.Run(ctx)
	defer stopper.Close()
	defer utterance.Close()
	defer filter.Close()

	go pump(ctx, stopper.Downstream, utterance.Upstream)

	stopper.Upstream <- types.Event{
		Kind: types.EventHumanInterimUtterance,
		Payload: types.OutputLine{
			Role:   "user",
			Text:   "明日",
			Final:  false,
			Source: "server-stt",
		},
	}
	if !waitForCurrentGeneration(store, 2) {
		t.Fatalf("generation after interim = %d, want 2", store.Current())
	}

	filter.Upstream <- types.Event{
		Kind:    types.EventScheduledItem,
		Payload: types.TimelineItem{GenerationID: 1, Kind: types.TimelineKindSpeech, Text: "old"},
	}
	assertNoPipelineEvent(t, filter.Downstream)

	stopper.Upstream <- types.Event{
		Kind: types.EventHumanUtterance,
		Payload: types.OutputLine{
			Role:   "user",
			Text:   "明日の予定",
			Final:  true,
			Source: "server-stt",
		},
	}
	evt := expect(t, utterance.Downstream)
	if evt.Kind != types.EventConversationCommitRequest {
		t.Fatalf("Kind = %s, want EventConversationCommitRequest", evt.Kind)
	}
	req := evt.Payload.(types.ConversationCommitRequest)
	if req.Text != "明日の予定" {
		t.Fatalf("Text = %q, want 明日の予定", req.Text)
	}
	if req.GenerationID != 2 {
		t.Fatalf("GenerationID = %d, want 2", req.GenerationID)
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

func waitForCurrentGeneration(store *generation.Store, want types.GenerationID) bool {
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if store.Current() == want {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return store.Current() == want
}

func assertNoPipelineEvent(t *testing.T, ch <-chan types.Event) {
	t.Helper()
	select {
	case evt := <-ch:
		t.Fatalf("unexpected event: %#v", evt)
	case <-time.After(20 * time.Millisecond):
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
