package utterancebuffer

import (
	"context"
	"testing"
	"time"

	"github.com/tetetratra/smart-speaker/internal/states/generation"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

func TestStageBuffersUtteranceAndAssignsGeneration(t *testing.T) {
	store := generation.NewStore()
	store.Next()
	st := NewStage(Config{Delay: 10 * time.Millisecond, Generation: store})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st.Run(ctx)
	defer st.Close()

	st.Upstream <- types.Event{Kind: types.EventHumanUtterance, Payload: types.OutputLine{Role: "user", Text: "えーっと"}}
	st.Upstream <- types.Event{Kind: types.EventHumanUtterance, Payload: types.OutputLine{Role: "user", Text: "明日の予定は"}}

	select {
	case evt := <-st.Downstream:
		if evt.Kind != types.EventConversationCommitRequest {
			t.Fatalf("Kind = %s, want EventConversationCommitRequest", evt.Kind)
		}
		req := evt.Payload.(types.ConversationCommitRequest)
		if req.Text != "えーっと 明日の予定は" {
			t.Fatalf("Text = %q", req.Text)
		}
		if req.GenerationID != 1 {
			t.Fatalf("GenerationID = %d, want 1", req.GenerationID)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting buffered utterance")
	}
}
