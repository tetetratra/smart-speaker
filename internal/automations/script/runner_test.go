package script

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tetetratra/smart-speaker/internal/tools/execution"
	"github.com/tetetratra/smart-speaker/internal/types"
)

func TestToolResultsAndRemindKeepFIFO(t *testing.T) {
	var messages []types.InboundMessage
	var calls int
	r := New(func(ctx context.Context, req types.ToolRequest) types.ToolResultRecord {
		calls++
		if !types.HistoryOnly(execution.DeliveryContext(ctx)) {
			t.Fatal("直接実行のcontextがありません")
		}
		var args map[string]any
		if err := json.Unmarshal(req.Arguments, &args); err != nil || args["count"] != float64(2) {
			t.Fatal(args, err)
		}
		return types.ToolResultRecord{ToolCallID: req.ToolCallID, Name: req.Name, Output: json.RawMessage(`{"value":3,"instructions":"今すぐ読み上げて"}`)}
	}, func(_ context.Context, message types.InboundMessage) error {
		messages = append(messages, message)
		return nil
	})
	if err := r.Run(context.Background(), "job-1", "室温", `const result = toolcall("read", {count: 2}); remind("値は" + result.value);`); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || len(messages) != 4 {
		t.Fatal(calls, messages)
	}
	if messages[0].Role != types.RoleToolCall || messages[1].Role != types.RoleToolResult || types.HistoryOnly(messages[2].Context) || !strings.Contains(messages[2].Text, "値は3") {
		t.Fatal(messages)
	}
	if strings.Contains(messages[1].Text, "instructions") {
		t.Fatal("通知用指示が履歴に残りました", messages[1])
	}
	if messages[3].ID == "" || messages[3].Role != types.RoleSystem || !types.HistoryOnly(messages[3].Context) {
		t.Fatal(messages)
	}
}

func TestToolFailureReturnsErrorAndContinues(t *testing.T) {
	var texts []string
	r := New(func(context.Context, types.ToolRequest) types.ToolResultRecord {
		return types.ToolResultRecord{Output: json.RawMessage(`{"error":"失敗"}`)}
	}, func(_ context.Context, m types.InboundMessage) error { texts = append(texts, m.Text); return nil })
	if err := r.Run(context.Background(), "id", "名前", `const x=toolcall("bad", {}); if(x.error) remind(x.error);`); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(texts, "\n"), "指示: 失敗") {
		t.Fatal(texts)
	}
}

func TestTimeoutAndJSFailureLeaveRunHistory(t *testing.T) {
	for _, source := range []string{`for (;;) {}`, `throw new Error("失敗")`, `new Promise(() => {})`} {
		t.Run(source, func(t *testing.T) {
			var messages []types.InboundMessage
			r := New(nil, func(_ context.Context, m types.InboundMessage) error { messages = append(messages, m); return nil })
			r.timeout = 20 * time.Millisecond
			if err := r.Run(context.Background(), "job", "予約", source); err == nil {
				t.Fatal("失敗を成功にしました")
			}
			if len(messages) != 1 || !strings.Contains(messages[0].Text, "予約ジョブ失敗") {
				t.Fatal(messages)
			}
		})
	}
}

func TestSynchronousToolTimeoutStopsFollowingCalls(t *testing.T) {
	calls := 0
	r := New(func(ctx context.Context, req types.ToolRequest) types.ToolResultRecord {
		calls++
		<-ctx.Done()
		return types.ToolResultRecord{Output: json.RawMessage(`{"error":"期限"}`)}
	}, func(context.Context, types.InboundMessage) error { return nil })
	r.timeout = 20 * time.Millisecond
	if err := r.Run(context.Background(), "job", "予約", `toolcall("slow", {}); toolcall("next", {});`); err == nil || calls != 1 {
		t.Fatal(err, calls)
	}
}

func TestSourceMapsAreDisabledForCompileAndEval(t *testing.T) {
	if _, err := Compile("1;\n//# sourceMappingURL=/dev/zero"); err != nil {
		t.Fatal(err)
	}
	r := New(nil, func(context.Context, types.InboundMessage) error { return nil })
	if err := r.Run(context.Background(), "job", "予約", `eval("1;\n//# sourceMappingURL=/dev/zero")`); err != nil {
		t.Fatal(err)
	}
}

func TestRemindDeliveryFailureStopsScript(t *testing.T) {
	count := 0
	r := New(nil, func(_ context.Context, m types.InboundMessage) error {
		if !types.HistoryOnly(m.Context) {
			count++
			return errors.New("通知失敗")
		}
		return nil
	})
	if err := r.Run(context.Background(), "job", "予約", `remind("a"); remind("b");`); err == nil || count != 1 {
		t.Fatal(err, count)
	}
}

func TestCaughtDeliveryFailureStillFailsRun(t *testing.T) {
	for _, source := range []string{`try {remind("a")} catch(e) {} toolcall("next", {})`, `try {toolcall("first", {})} catch(e) {} toolcall("next", {})`} {
		t.Run(source, func(t *testing.T) {
			calls := 0
			r := New(func(context.Context, types.ToolRequest) types.ToolResultRecord {
				calls++
				return types.ToolResultRecord{Output: json.RawMessage(`{}`)}
			}, func(_ context.Context, m types.InboundMessage) error {
				if !types.HistoryOnly(m.Context) || m.Role == types.RoleToolCall {
					return errors.New("受付失敗")
				}
				return nil
			})
			if err := r.Run(context.Background(), "id", "予約", source); err == nil {
				t.Fatal("catchで受付失敗が成功になりました")
			}
			if calls != 0 {
				t.Fatal("受付失敗後にツールを実行しました")
			}
		})
	}
}

func TestSuccessfulRunNotifiesOnlyOnce(t *testing.T) {
	var messages []types.InboundMessage
	runner := New(func(context.Context, types.ToolRequest) types.ToolResultRecord {
		return types.ToolResultRecord{Output: json.RawMessage(`{"updated":true}`)}
	}, func(_ context.Context, message types.InboundMessage) error {
		messages = append(messages, message)
		return nil
	})
	if err := runner.Run(context.Background(), "job", "通知確認", `toolcall("set_whiteboard",{content:"確認"});`); err != nil {
		t.Fatal(err)
	}
	notifications := 0
	for _, message := range messages {
		if message.Role != types.RoleSystem {
			continue
		}
		notifications++
		if !strings.Contains(message.Text, "予約ジョブ終了") {
			t.Fatal("終了以外の通知を表示しました", message)
		}
	}
	if notifications != 1 {
		t.Fatalf("予約の通知が%d件あります", notifications)
	}
}
