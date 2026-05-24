package scheduler

import (
	"context"
	"testing"
	"time"

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
