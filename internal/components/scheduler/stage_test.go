package scheduler

import (
	"context"
	"testing"
	"time"

	pbspeed "github.com/tetetratra/smart-speaker/internal/states/playbackspeed"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

func TestStageSchedulesSpeechWaitAndToolInOrder(t *testing.T) {
	st := NewStage(Config{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st.Run(ctx)
	defer st.Close()

	st.Upstream <- types.Event{Kind: types.EventPlayableSpeech, Payload: types.PlayableSpeech{GenerationID: 1, Text: "確認するね", DurationSeconds: 0.01}}
	st.Upstream <- types.Event{Kind: types.EventTimelineItem, Payload: types.TimelineItem{Kind: types.TimelineKindWait, GenerationID: 1, Sec: 0.01}}
	st.Upstream <- types.Event{Kind: types.EventTimelineItem, Payload: types.TimelineItem{Kind: types.TimelineKindTool, GenerationID: 1, SequenceID: "3", ToolName: "get_temp"}}

	first := expectScheduled(t, st.Downstream)
	if speech, ok := first.Payload.(types.PlayableSpeech); !ok || speech.Text != "確認するね" {
		t.Fatalf("first payload = %#v", first.Payload)
	}
	second := expectScheduled(t, st.Downstream)
	if req, ok := second.Payload.(types.ToolRequest); !ok || req.Name != "get_temp" {
		t.Fatalf("second payload = %#v", second.Payload)
	}
}

func TestStageEmitsPlaybackEndAfterTimelineEnd(t *testing.T) {
	st := NewStage(Config{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st.Run(ctx)
	defer st.Close()

	st.Upstream <- types.Event{Kind: types.EventPlayableSpeech, Payload: types.PlayableSpeech{GenerationID: 1, Text: "確認するね", DurationSeconds: 0.01}}
	st.Upstream <- types.Event{Kind: types.EventTimelineItem, Payload: types.TimelineItem{Kind: types.TimelineKindWait, GenerationID: 1, Sec: 0.01}}
	st.Upstream <- types.Event{Kind: types.EventAgentTimelineEnd, Payload: types.AgentTimelineEnd{GenerationID: 1}}

	expectScheduled(t, st.Downstream)

	select {
	case evt := <-st.Downstream:
		if evt.Kind != types.EventAgentSpeechPlaybackEnd {
			t.Fatalf("Kind = %s, want EventAgentSpeechPlaybackEnd", evt.Kind)
		}
		end := evt.Payload.(types.AgentSpeechPlaybackEnd)
		if end.GenerationID != 1 {
			t.Fatalf("GenerationID = %d, want 1", end.GenerationID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting agent speech playback end")
	}
}

func TestStageEmitsPlaybackEndForEmptyTimeline(t *testing.T) {
	st := NewStage(Config{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st.Run(ctx)
	defer st.Close()

	st.Upstream <- types.Event{Kind: types.EventAgentTimelineEnd, Payload: types.AgentTimelineEnd{GenerationID: 3}}

	select {
	case evt := <-st.Downstream:
		if evt.Kind != types.EventAgentSpeechPlaybackEnd {
			t.Fatalf("Kind = %s, want EventAgentSpeechPlaybackEnd", evt.Kind)
		}
		end := evt.Payload.(types.AgentSpeechPlaybackEnd)
		if end.GenerationID != 3 {
			t.Fatalf("GenerationID = %d, want 3", end.GenerationID)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting agent speech playback end")
	}
}

func TestStageAdjustsTimelineWaitByPlaybackSpeed(t *testing.T) {
	store := pbspeed.NewStore()
	store.SetSpeed(2)
	st := NewStage(Config{SpeedStore: store})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st.Run(ctx)
	defer st.Close()

	st.Upstream <- types.Event{Kind: types.EventTimelineItem, Payload: types.TimelineItem{Kind: types.TimelineKindWait, GenerationID: 1, Sec: 0.18}}
	st.Upstream <- types.Event{Kind: types.EventTimelineItem, Payload: types.TimelineItem{Kind: types.TimelineKindTool, GenerationID: 1, SequenceID: "2", ToolName: "get_temp"}}

	select {
	case evt := <-st.Downstream:
		req, ok := evt.Payload.(types.ToolRequest)
		if evt.Kind != types.EventScheduledItem || !ok || req.Name != "get_temp" {
			t.Fatalf("event = %#v", evt)
		}
	case <-time.After(150 * time.Millisecond):
		t.Fatal("timeout waiting adjusted wait to complete")
	}
}

func TestStageDoesNotAdjustSpeechDurationByPlaybackSpeed(t *testing.T) {
	store := pbspeed.NewStore()
	store.SetSpeed(3)
	st := NewStage(Config{SpeedStore: store})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st.Run(ctx)
	defer st.Close()

	st.Upstream <- types.Event{Kind: types.EventPlayableSpeech, Payload: types.PlayableSpeech{GenerationID: 1, Text: "確認するね", DurationSeconds: 0.12}}
	st.Upstream <- types.Event{Kind: types.EventTimelineItem, Payload: types.TimelineItem{Kind: types.TimelineKindTool, GenerationID: 1, SequenceID: "2", ToolName: "get_temp"}}

	expectScheduled(t, st.Downstream)

	select {
	case evt := <-st.Downstream:
		t.Fatalf("tool emitted before speech duration elapsed: %#v", evt)
	case <-time.After(80 * time.Millisecond):
	}

	second := expectScheduled(t, st.Downstream)
	if req, ok := second.Payload.(types.ToolRequest); !ok || req.Name != "get_temp" {
		t.Fatalf("second payload = %#v", second.Payload)
	}
}

func expectScheduled(t *testing.T, ch <-chan types.Event) types.Event {
	t.Helper()
	select {
	case evt := <-ch:
		if evt.Kind != types.EventScheduledItem {
			t.Fatalf("Kind = %s, want EventScheduledItem", evt.Kind)
		}
		return evt
	case <-time.After(time.Second):
		t.Fatal("timeout waiting scheduled item")
		return types.Event{}
	}
}
