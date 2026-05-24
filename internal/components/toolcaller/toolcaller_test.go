package toolcaller

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	types "github.com/tetetratra/smart-speaker/internal/types"
)

type fakeCommitter struct {
	ch chan types.ToolResultRecord
}

func (f *fakeCommitter) CommitToolResult(ctx context.Context, result types.ToolResultRecord) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case f.ch <- result:
		return nil
	}
}

func TestStageCommitsUnknownToolResult(t *testing.T) {
	committer := &fakeCommitter{ch: make(chan types.ToolResultRecord, 1)}
	st := NewStage(nil, committer)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st.Run(ctx)
	defer st.Close()

	st.Upstream <- types.Event{Kind: types.EventToolRequest, Payload: types.ToolRequest{
		ToolCallID:   "call-1",
		Name:         "unknown",
		Arguments:    json.RawMessage(`{}`),
		GenerationID: 3,
	}}

	select {
	case result := <-committer.ch:
		if result.Name != "unknown" {
			t.Fatalf("Name = %q", result.Name)
		}
		if result.GenerationID != 3 {
			t.Fatalf("GenerationID = %d", result.GenerationID)
		}
		if len(result.Output) == 0 {
			t.Fatal("Output is empty")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting tool result")
	}
	select {
	case evt := <-st.Downstream:
		t.Fatalf("unexpected downstream event: %s", evt.Kind)
	default:
	}
}
