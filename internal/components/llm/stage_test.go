package llm

import (
	"context"
	"testing"
	"time"

	"smart-speaker/internal/states/conversationhistory"
	types "smart-speaker/internal/types"
)

type fakeClient struct {
	calls     int
	responses [][]string
}

func (f *fakeClient) CreateResponseStream(ctx context.Context, messages []types.ChatMessage, systemContent string, onLine func(string) error) error {
	idx := f.calls
	f.calls++
	if idx >= len(f.responses) {
		idx = len(f.responses) - 1
	}
	for _, line := range f.responses[idx] {
		if err := onLine(line); err != nil {
			return err
		}
	}
	return nil
}

func TestStageRetriesInvalidResponse(t *testing.T) {
	history := conversationhistory.NewStore()
	history.Append(types.ConversationRecord{Role: types.RoleUser, Text: "温度見て", GenerationID: 1})
	client := &fakeClient{responses: [][]string{
		{`{"type":"speech","text":""}`},
		{`{"type":"speech","text":"確認するね"}`},
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
	select {
	case evt := <-st.Downstream:
		if evt.Kind != types.EventTimelineItem {
			t.Fatalf("Kind = %s", evt.Kind)
		}
		item := evt.Payload.(types.TimelineItem)
		if item.Text != "確認するね" {
			t.Fatalf("Text = %q", item.Text)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting timeline item")
	}
	if client.calls != 2 {
		t.Fatalf("calls = %d, want 2", client.calls)
	}
}
