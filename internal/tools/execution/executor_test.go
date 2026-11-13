package execution

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/tetetratra/smart-speaker/internal/states/toolhistory"
	"github.com/tetetratra/smart-speaker/internal/tools"
	"github.com/tetetratra/smart-speaker/internal/types"
)

type testTool struct {
	calls int
	run   func(context.Context, map[string]any) (map[string]any, error)
}

func (t *testTool) Name() string { return "test" }
func (t *testTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	t.calls++
	return t.run(ctx, args)
}

func TestExecutorSharesContextArgumentsAndErrorHistory(t *testing.T) {
	raw := json.RawMessage(`{"delivery":"history_only","future":{"value":2}}`)
	tool := &testTool{run: func(ctx context.Context, args map[string]any) (map[string]any, error) {
		if string(DeliveryContext(ctx)) != string(raw) || args["n"] != float64(3) {
			t.Fatal(args, string(DeliveryContext(ctx)))
		}
		return nil, errors.New("失敗")
	}}
	store := toolhistory.NewStore(10)
	result := New(map[string]tools.Handler{"test": tool}, store).Execute(WithDeliveryContext(context.Background(), raw), types.ToolRequest{ToolCallID: "call", Name: "test", Arguments: json.RawMessage(`{"n":3}`)})
	if string(result.Output) != `{"error":"失敗"}` || tool.calls != 1 {
		t.Fatal(result, tool.calls)
	}
	if entries := store.Snapshot(); len(entries) != 1 || entries[0].Status != "failed" {
		t.Fatal(entries)
	}
}

func TestExecutorRejectsCanceledAndInvalidCallsBeforeSideEffect(t *testing.T) {
	tool := &testTool{run: func(context.Context, map[string]any) (map[string]any, error) {
		t.Fatal("実行してはいけません")
		return nil, nil
	}}
	executor := New(map[string]tools.Handler{"test": tool}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, test := range []struct {
		ctx  context.Context
		args string
	}{{ctx, `{}`}, {context.Background(), `null`}, {context.Background(), `[]`}, {context.Background(), `{"n":`}} {
		result := executor.Execute(test.ctx, types.ToolRequest{Name: "test", Arguments: json.RawMessage(test.args)})
		var payload map[string]any
		if json.Unmarshal(result.Output, &payload) != nil || payload["error"] == nil {
			t.Fatal(string(result.Output))
		}
	}
	if tool.calls != 0 {
		t.Fatal(tool.calls)
	}
}

func TestDeliveryContextDoesNotShareMutableBytes(t *testing.T) {
	raw := json.RawMessage(`{"future":1}`)
	ctx := WithDeliveryContext(context.Background(), raw)
	raw[0] = '['
	copy := DeliveryContext(ctx)
	copy[0] = '['
	if got := string(DeliveryContext(ctx)); got != `{"future":1}` {
		t.Fatal(got)
	}
}

type titledTestTool struct{ *testTool }

func (t *titledTestTool) Title(args map[string]any) string { return args["subject"].(string) }

func TestTitleIsVisibleWhileRunningAndPreservedAfterCompletion(t *testing.T) {
	store := toolhistory.NewStore(10)
	tool := &titledTestTool{&testTool{run: func(_ context.Context, args map[string]any) (map[string]any, error) {
		entry := store.Snapshot()[0]
		if entry.Name != "test" || entry.Title != "調査対象" || entry.Status != "running" {
			t.Fatal(entry)
		}
		return map[string]any{"done": true}, nil
	}}}
	executor := New(map[string]tools.Handler{"test": tool}, store)
	executor.Execute(context.Background(), types.ToolRequest{Name: "test", Arguments: json.RawMessage(`{"subject":"調査対象"}`)})
	entry := store.Snapshot()[0]
	if entry.Title != "調査対象" || entry.Status != "succeeded" {
		t.Fatal(entry)
	}
}
