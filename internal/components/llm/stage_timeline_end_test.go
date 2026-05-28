package llm

import (
	"context"
	"testing"
	"time"

	"github.com/tetetratra/smart-speaker/internal/states/conversationhistory"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

func TestStageEmitsAgentTimelineEndAfterItems(t *testing.T) {
	history := conversationhistory.NewStore()
	client := &fakeClient{responses: []string{
		`{"items":[{"type":"speech","text":"確認するね"},{"type":"wait","sec":0.01}]}`,
	}}
	st, err := NewStage(Config{History: history, Client: client})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st.Run(ctx)
	defer st.Close()

	st.Upstream <- types.Event{Kind: types.EventLLMRequest, Payload: types.LLMRequest{RequestID: "req-1", GenerationID: 1}}

	var items int
	var sawEnd bool
	deadline := time.After(2 * time.Second)
	for items < 2 || !sawEnd {
		select {
		case evt := <-st.Downstream:
			switch evt.Kind {
			case types.EventTimelineItem:
				items++
			case types.EventAgentTimelineEnd:
				end := evt.Payload.(types.AgentTimelineEnd)
				if end.GenerationID != 1 {
					t.Fatalf("GenerationID = %d, want 1", end.GenerationID)
				}
				sawEnd = true
			default:
				t.Fatalf("unexpected Kind = %s", evt.Kind)
			}
		case <-deadline:
			t.Fatalf("timeout: items=%d sawEnd=%v", items, sawEnd)
		}
	}
}

func TestStageEmitsAgentTimelineEndForEmptyTimeline(t *testing.T) {
	client := &fakeClient{responses: []string{`{"items":[]}`}}
	st, err := NewStage(Config{Client: client})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st.Run(ctx)
	defer st.Close()

	st.Upstream <- types.Event{Kind: types.EventLLMRequest, Payload: types.LLMRequest{RequestID: "req-1", GenerationID: 2}}

	select {
	case evt := <-st.Downstream:
		if evt.Kind != types.EventAgentTimelineEnd {
			t.Fatalf("Kind = %s, want EventAgentTimelineEnd", evt.Kind)
		}
		end := evt.Payload.(types.AgentTimelineEnd)
		if end.GenerationID != 2 {
			t.Fatalf("GenerationID = %d, want 2", end.GenerationID)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting agent timeline end")
	}
}
