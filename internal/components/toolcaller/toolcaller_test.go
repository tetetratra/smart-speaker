package toolcaller

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	types "github.com/tetetratra/smart-speaker/internal/types"
)

func TestStageEmitsUnknownToolResultCommitRequest(t *testing.T) {
	st := NewStage(nil)
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
	case evt := <-st.Downstream:
		if evt.Kind != types.EventConversationCommitRequest {
			t.Fatalf("Kind = %s, want EventConversationCommitRequest", evt.Kind)
		}
		req, ok := evt.Payload.(types.ConversationCommitRequest)
		if !ok {
			t.Fatalf("Payload = %T, want ConversationCommitRequest", evt.Payload)
		}
		if req.Role != types.RoleToolResult {
			t.Fatalf("Role = %s, want tool_result", req.Role)
		}
		if req.ToolResult == nil {
			t.Fatal("ToolResult is nil")
		}
		if req.ToolResult.Name != "unknown" {
			t.Fatalf("Name = %q", req.ToolResult.Name)
		}
		if req.GenerationID != 3 || req.ToolResult.GenerationID != 3 {
			t.Fatalf("GenerationID = %d / %d, want 3", req.GenerationID, req.ToolResult.GenerationID)
		}
		if len(req.ToolResult.Output) == 0 {
			t.Fatal("Output is empty")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting tool result commit request")
	}
}
