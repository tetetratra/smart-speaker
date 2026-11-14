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
	var messages []types.ConversationInput
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
	}, func(_ context.Context, message types.ConversationInput) error {
		messages = append(messages, message)
		return nil
	})
	if err := r.Run(context.Background(), "job-1", "室温", `const result = toolcall("read", {count: 2}); remind("値は" + result.value);`); err != nil {
		t.Fatal(err)
	}
	for _, message := range messages {
		// ツールの呼び出しと結果のタイトルはツールが決めるので、予約の名前は通知にだけ付ける。
		if message.Record.Source != Source || (message.Record.ToolName == "" && message.Record.Title != "室温") {
			t.Fatal("予約の名前とタイトルが引き継がれていません", message)
		}
		var metadata map[string]string
		if json.Unmarshal(deliveryMetadata(message), &metadata) != nil || metadata["automation_id"] != "job-1" || metadata["run_id"] == "" {
			t.Fatal("予約の識別情報が引き継がれていません", message)
		}
	}
	if calls != 1 || len(messages) != 4 {
		t.Fatal(calls, messages)
	}
	if call, result := messages[0], messages[1]; call.Record.ToolName != "read" || call.Record.CallID == "" || call.Record.Arguments != `{"count":2}` || result.Record.CallID != call.Record.CallID || strings.Contains(result.Record.Output, "instructions") {
		t.Fatal("ツールの表示用の項目がありません", messages)
	}
	if messages[0].Record.Role != types.RoleToolCall || messages[1].Record.Role != types.RoleToolResult || messages[2].ModelHandling == types.ModelHandlingAddToContext || !strings.Contains(messages[2].Record.Text, "値は3") {
		t.Fatal(messages)
	}
	if strings.Contains(messages[1].Record.Text, "instructions") {
		t.Fatal("通知用指示が履歴に残りました", messages[1])
	}
	if messages[3].Record.ID == "" || messages[3].Record.Role != types.RoleSystem || messages[3].ModelHandling != types.ModelHandlingAddToContext {
		t.Fatal(messages)
	}
}

func TestToolFailureReturnsErrorAndContinues(t *testing.T) {
	var texts []string
	r := New(func(context.Context, types.ToolRequest) types.ToolResultRecord {
		return types.ToolResultRecord{Output: json.RawMessage(`{"error":"失敗"}`)}
	}, func(_ context.Context, m types.ConversationInput) error {
		texts = append(texts, m.Record.Text)
		return nil
	})
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
			var messages []types.ConversationInput
			r := New(nil, func(_ context.Context, m types.ConversationInput) error { messages = append(messages, m); return nil })
			r.timeout = 20 * time.Millisecond
			if err := r.Run(context.Background(), "job", "予約", source); err == nil {
				t.Fatal("失敗を成功にしました")
			}
			if len(messages) != 1 || !strings.Contains(messages[0].Record.Text, "予約ジョブ失敗") {
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
	}, func(context.Context, types.ConversationInput) error { return nil })
	r.timeout = 20 * time.Millisecond
	if err := r.Run(context.Background(), "job", "予約", `toolcall("slow", {}); toolcall("next", {});`); err == nil || calls != 1 {
		t.Fatal(err, calls)
	}
}

func TestSourceMapsAreDisabledForCompileAndEval(t *testing.T) {
	if _, err := Compile("1;\n//# sourceMappingURL=/dev/zero"); err != nil {
		t.Fatal(err)
	}
	r := New(nil, func(context.Context, types.ConversationInput) error { return nil })
	if err := r.Run(context.Background(), "job", "予約", `eval("1;\n//# sourceMappingURL=/dev/zero")`); err != nil {
		t.Fatal(err)
	}
}

func TestRemindDeliveryFailureStopsScript(t *testing.T) {
	count := 0
	r := New(nil, func(_ context.Context, m types.ConversationInput) error {
		if m.ModelHandling != types.ModelHandlingAddToContext {
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
			}, func(_ context.Context, m types.ConversationInput) error {
				if m.ModelHandling != types.ModelHandlingAddToContext || m.Record.Role == types.RoleToolCall {
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
	var messages []types.ConversationInput
	runner := New(func(context.Context, types.ToolRequest) types.ToolResultRecord {
		return types.ToolResultRecord{Output: json.RawMessage(`{"updated":true}`)}
	}, func(_ context.Context, message types.ConversationInput) error {
		messages = append(messages, message)
		return nil
	})
	if err := runner.Run(context.Background(), "job", "通知確認", `toolcall("set_whiteboard",{content:"確認"});`); err != nil {
		t.Fatal(err)
	}
	notifications := 0
	for _, message := range messages {
		if message.Record.Role != types.RoleSystem {
			continue
		}
		notifications++
		if !strings.Contains(message.Record.Text, "予約ジョブ終了") {
			t.Fatal("終了以外の通知を表示しました", message)
		}
	}
	if notifications != 1 {
		t.Fatalf("予約の通知が%d件あります", notifications)
	}
}

// deliveryMetadata は、届いた入力に残った通知の識別情報（delivery を除いたもの）を返す。
func deliveryMetadata(m types.ConversationInput) json.RawMessage {
	raw, _ := m.Record.Metadata["context"].(string)
	return json.RawMessage(raw)
}
