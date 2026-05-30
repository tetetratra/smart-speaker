package toolcaller

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/tetetratra/smart-speaker/internal/tools"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

type stubHandler struct {
	name string
	out  map[string]any
	err  error
}

func (h stubHandler) Name() string { return h.name }

func (h stubHandler) Run(args map[string]any) (map[string]any, error) {
	if h.err != nil {
		return nil, h.err
	}
	return h.out, nil
}

func TestStageEmitsUnknownToolResultCommitRequest(t *testing.T) {
	st := NewStage(nil, nil)
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
			t.Fatalf("Payload type = %T, want ConversationCommitRequest", evt.Payload)
		}
		if req.Role != types.RoleToolResult {
			t.Fatalf("Role = %q, want tool_result", req.Role)
		}
		if req.Source != "unknown" {
			t.Fatalf("Source = %q, want unknown", req.Source)
		}
		if req.ToolResult == nil {
			t.Fatal("ToolResult is nil")
		}
		result := req.ToolResult
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
		t.Fatal("timeout waiting commit request")
	}
}

func TestStageSuppressesSuccessfulWriteToolResult(t *testing.T) {
	st := NewStage(map[string]tools.Handler{
		"set_whiteboard": stubHandler{
			name: "set_whiteboard",
			out:  map[string]any{"updated": true},
		},
	}, map[string]string{
		"set_whiteboard": tools.ToolModeWrite,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st.Run(ctx)
	defer st.Close()

	st.Upstream <- types.Event{Kind: types.EventToolRequest, Payload: types.ToolRequest{
		ToolCallID:   "call-1",
		Name:         "set_whiteboard",
		Arguments:    json.RawMessage(`{"content":"メモ"}`),
		GenerationID: 1,
	}}

	select {
	case evt := <-st.Downstream:
		t.Fatalf("unexpected downstream event: %+v", evt)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestStageEmitsWriteToolResultOnError(t *testing.T) {
	st := NewStage(map[string]tools.Handler{
		"set_whiteboard": stubHandler{
			name: "set_whiteboard",
			err:  errors.New("failed"),
		},
	}, map[string]string{
		"set_whiteboard": tools.ToolModeWrite,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st.Run(ctx)
	defer st.Close()

	st.Upstream <- types.Event{Kind: types.EventToolRequest, Payload: types.ToolRequest{
		ToolCallID:   "call-1",
		Name:         "set_whiteboard",
		Arguments:    json.RawMessage(`{"content":"メモ"}`),
		GenerationID: 1,
	}}

	select {
	case evt := <-st.Downstream:
		req := evt.Payload.(types.ConversationCommitRequest)
		if req.ToolResult == nil {
			t.Fatal("ToolResult is nil")
		}
		if !json.Valid(req.ToolResult.Output) {
			t.Fatalf("Output = %q", req.ToolResult.Output)
		}
		var out map[string]any
		if err := json.Unmarshal(req.ToolResult.Output, &out); err != nil {
			t.Fatal(err)
		}
		if out["error"] != "failed" {
			t.Fatalf("output = %#v", out)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting commit request")
	}
}

func TestStageEmitsReadToolResultOnSuccess(t *testing.T) {
	st := NewStage(map[string]tools.Handler{
		"web_search": stubHandler{
			name: "web_search",
			out:  map[string]any{"result": "ok"},
		},
	}, map[string]string{
		"web_search": tools.ToolModeRead,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st.Run(ctx)
	defer st.Close()

	st.Upstream <- types.Event{Kind: types.EventToolRequest, Payload: types.ToolRequest{
		ToolCallID:   "call-1",
		Name:         "web_search",
		Arguments:    json.RawMessage(`{"query":"今日の天気を教えて"}`),
		GenerationID: 1,
	}}

	select {
	case evt := <-st.Downstream:
		req := evt.Payload.(types.ConversationCommitRequest)
		if req.ToolResult == nil {
			t.Fatal("ToolResult is nil")
		}
		var out map[string]any
		if err := json.Unmarshal(req.ToolResult.Output, &out); err != nil {
			t.Fatal(err)
		}
		if out["result"] != "ok" {
			t.Fatalf("output = %#v", out)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting commit request")
	}
}
