package toolcaller

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/tetetratra/smart-speaker/internal/states/toolhistory"
	"github.com/tetetratra/smart-speaker/internal/tools"
	"github.com/tetetratra/smart-speaker/internal/tools/execution"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

type stubHandler struct {
	name string
	out  map[string]any
	err  error
}

func (h stubHandler) Name() string { return h.name }

func (h stubHandler) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
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

func TestStagePreservesSuccessfulWriteToolResult(t *testing.T) {
	st := NewStage(map[string]tools.Handler{
		"set_whiteboard": stubHandler{
			name: "set_whiteboard",
			out:  map[string]any{"updated": true},
		},
	}, nil)
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
			t.Fatal("成功した操作結果が保存されない")
		}
	case <-time.After(time.Second):
		t.Fatal("結果が届かない")
	}
}

func TestStageEmitsWriteToolResultOnError(t *testing.T) {
	st := NewStage(map[string]tools.Handler{
		"set_whiteboard": stubHandler{
			name: "set_whiteboard",
			err:  errors.New("failed"),
		},
	}, nil)
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
	}, nil)
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

func TestDuplicateCallIsNotExecutedTwice(t *testing.T) {
	handler := &countingHandler{}
	st := NewStage(map[string]tools.Handler{"count": handler}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st.Run(ctx)
	defer st.Close()
	for i := 0; i < 2; i++ {
		st.Upstream <- types.Event{Kind: types.EventToolRequest, Payload: types.ToolRequest{ToolCallID: "same", Name: "count", Arguments: json.RawMessage(`{}`)}}
		select {
		case <-st.Downstream:
		case <-time.After(time.Second):
			t.Fatal("結果が届かない")
		}
	}
	if handler.calls != 1 {
		t.Fatal(handler.calls)
	}
}
func TestInvalidArgumentsDoNotExecuteTool(t *testing.T) {
	handler := &countingHandler{}
	executor := execution.New(map[string]tools.Handler{"count": handler}, nil)
	result := executor.Execute(context.Background(), types.ToolRequest{Name: "count", Arguments: json.RawMessage(`{"broken"`)})
	if handler.calls != 0 || !json.Valid(result.Output) {
		t.Fatal(handler.calls, string(result.Output))
	}
}

type countingHandler struct{ calls int }

func (h *countingHandler) Name() string { return "count" }
func (h *countingHandler) Run(context.Context, map[string]any) (map[string]any, error) {
	h.calls++
	return map[string]any{"ok": true}, nil
}

type blockingHandler struct{ started, release chan struct{} }

func (h *blockingHandler) Name() string { return "blocked" }
func (h *blockingHandler) Run(context.Context, map[string]any) (map[string]any, error) {
	close(h.started)
	<-h.release
	return map[string]any{"result": "結果の全文"}, nil
}

func TestHistoryTracksRunningResultAndDuplicateCall(t *testing.T) {
	history := toolhistory.NewStore(200)
	handler := &blockingHandler{started: make(chan struct{}), release: make(chan struct{})}
	stage := NewStage(map[string]tools.Handler{"blocked": handler}, history)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	stage.Run(ctx)
	defer stage.Close()
	req := types.ToolRequest{ToolCallID: "call-id", Name: "blocked", Arguments: json.RawMessage(`{"text":"引数の全文"}`), GenerationID: 4}
	stage.Upstream <- types.Event{Kind: types.EventToolRequest, Payload: req}
	select {
	case <-handler.started:
	case <-ctx.Done():
		t.Fatal("ツールが開始されませんでした")
	}
	entries := history.Snapshot()
	if len(entries) != 1 || entries[0].Status != "running" || entries[0].Arguments != string(req.Arguments) {
		t.Fatal(entries)
	}
	close(handler.release)
	select {
	case <-stage.Downstream:
	case <-ctx.Done():
		t.Fatal("結果が届きませんでした")
	}
	stage.Upstream <- types.Event{Kind: types.EventToolRequest, Payload: req}
	select {
	case <-stage.Downstream:
	case <-ctx.Done():
		t.Fatal("再送の結果が届きませんでした")
	}
	entries = history.Snapshot()
	if len(entries) != 1 || entries[0].Status != "succeeded" || entries[0].FinishedAt.IsZero() || entries[0].Output != `{"result":"結果の全文"}` {
		t.Fatal(entries)
	}
}

func TestHistoryRecordsFailuresIncludingMalformedArguments(t *testing.T) {
	for _, req := range []types.ToolRequest{
		{ToolCallID: "invalid", Name: "count", Arguments: json.RawMessage(`{"broken"`)},
		{ToolCallID: "unknown", Name: "unknown", Arguments: json.RawMessage(`{}`)},
		{ToolCallID: "failed", Name: "failed", Arguments: json.RawMessage(`{}`)},
	} {
		t.Run(req.ToolCallID, func(t *testing.T) {
			history := toolhistory.NewStore(200)
			stage := NewStage(map[string]tools.Handler{"failed": stubHandler{err: errors.New("失敗の詳細")}}, history)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			stage.Run(ctx)
			defer stage.Close()
			stage.Upstream <- types.Event{Kind: types.EventToolRequest, Payload: req}
			select {
			case <-stage.Downstream:
			case <-ctx.Done():
				t.Fatal("結果が届きませんでした")
			}
			if history.Snapshot()[0].Status != "failed" {
				t.Fatal(history.Snapshot())
			}
		})
	}
}

// 時間のかかるツールの実行中でも、別のツールを実行して結果を先に返す。
// 実行中の call_id が再び届いても、二重には実行しない。
func TestSlowToolDoesNotBlockOtherCalls(t *testing.T) {
	slow := &blockingHandler{started: make(chan struct{}), release: make(chan struct{})}
	fast := &countingHandler{}
	st := NewStage(map[string]tools.Handler{"blocked": slow, "count": fast}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	st.Run(ctx)
	defer st.Close()
	st.Upstream <- types.Event{Kind: types.EventToolRequest, Payload: types.ToolRequest{ToolCallID: "slow", Name: "blocked"}}
	<-slow.started
	st.Upstream <- types.Event{Kind: types.EventToolRequest, Payload: types.ToolRequest{ToolCallID: "slow", Name: "blocked"}}
	st.Upstream <- types.Event{Kind: types.EventToolRequest, Payload: types.ToolRequest{ToolCallID: "fast", Name: "count"}}
	select {
	case evt := <-st.Downstream:
		if id := evt.Payload.(types.ConversationCommitRequest).ToolResult.ToolCallID; id != "fast" {
			t.Fatal(id)
		}
	case <-ctx.Done():
		t.Fatal("実行中のツールに別のツールが待たされました")
	}
	close(slow.release)
	select {
	case evt := <-st.Downstream:
		if id := evt.Payload.(types.ConversationCommitRequest).ToolResult.ToolCallID; id != "slow" {
			t.Fatal(id)
		}
	case <-ctx.Done():
		t.Fatal("時間のかかるツールの結果が届きません")
	}
	select {
	case evt := <-st.Downstream:
		t.Fatal("実行中に再送された呼び出しの結果が重複しました", evt)
	case <-time.After(50 * time.Millisecond):
	}
}
