package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/tetetratra/smart-speaker/internal/components/conversationcommitter"
	"github.com/tetetratra/smart-speaker/internal/components/generationfilter"
	"github.com/tetetratra/smart-speaker/internal/components/router"
	"github.com/tetetratra/smart-speaker/internal/components/scheduler"
	"github.com/tetetratra/smart-speaker/internal/components/toolcaller"
	"github.com/tetetratra/smart-speaker/internal/states/conversationhistory"
	"github.com/tetetratra/smart-speaker/internal/states/generation"
	"github.com/tetetratra/smart-speaker/internal/tools"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

type fakeTool struct{}

func (fakeTool) Name() string { return "get_temp" }

func (fakeTool) Run(args map[string]any) (map[string]any, error) {
	return map[string]any{"temp": 29}, nil
}

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

func TestToolCallerCommitsToolResultThroughGraphEvent(t *testing.T) {
	history := conversationhistory.NewStore()
	store := generation.NewStore()
	store.Next()
	store.Next()
	tool := toolcaller.NewStage(map[string]tools.Handler{"get_temp": fakeTool{}})
	committer := conversationcommitter.NewStage(conversationcommitter.Config{History: history, Generation: store})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tool.Run(ctx)
	committer.Run(ctx)
	defer tool.Close()
	defer committer.Close()

	go pumpKind(ctx, tool.Downstream, committer.Upstream, types.EventConversationCommitRequest)

	tool.Upstream <- types.Event{Kind: types.EventToolRequest, Payload: types.ToolRequest{
		ToolCallID:   "call-1",
		Name:         "get_temp",
		Arguments:    []byte(`{}`),
		GenerationID: 1,
	}}

	expectKind(t, committer.Downstream, types.EventRealtimeOutput)
	expectKind(t, committer.Downstream, types.EventLLMRequest)
	records := history.Snapshot()
	if len(records) != 1 {
		t.Fatalf("history len = %d, want 1", len(records))
	}
	if records[0].Role != types.RoleToolResult {
		t.Fatalf("Role = %s, want tool_result", records[0].Role)
	}
	if records[0].Metadata["stale"] != true {
		t.Fatalf("stale = %v, want true", records[0].Metadata["stale"])
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

func pumpKind(ctx context.Context, in <-chan types.Event, out chan<- types.Event, kind types.EventKind) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-in:
			if !ok {
				return
			}
			if evt.Kind != kind {
				continue
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

func expectKind(t *testing.T, ch <-chan types.Event, kind types.EventKind) types.Event {
	t.Helper()
	evt := expect(t, ch)
	if evt.Kind != kind {
		t.Fatalf("Kind = %s, want %s", evt.Kind, kind)
	}
	return evt
}
