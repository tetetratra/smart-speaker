package script

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRunRemindAndToolCall(t *testing.T) {
	var reminded []string
	var calls []string
	funcs := Funcs{
		Remind: func(_ context.Context, text string) error { reminded = append(reminded, text); return nil },
		ToolCall: func(_ context.Context, name string, args map[string]any) (json.RawMessage, error) {
			data, _ := json.Marshal(args)
			calls = append(calls, name+" "+string(data))
			return json.RawMessage(`{"temperature":30}`), nil
		},
	}
	src := `
const r = toolcall("get_temperature", {device: "Hub2"});
if (r.temperature > 28) remind("暑いのでエアコンをつけるか聞いて");
toolcall("light_on");
`
	if err := Run(context.Background(), src, funcs, time.Second); err != nil {
		t.Fatal(err)
	}
	if want := []string{"暑いのでエアコンをつけるか聞いて"}; !reflect.DeepEqual(reminded, want) {
		t.Fatalf("remind = %v, want %v", reminded, want)
	}
	if want := []string{`get_temperature {"device":"Hub2"}`, `light_on {}`}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("toolcall = %v, want %v", calls, want)
	}
}

func TestRunToolCallErrorCanBeCaught(t *testing.T) {
	var reminded []string
	funcs := Funcs{
		Remind: func(_ context.Context, text string) error { reminded = append(reminded, text); return nil },
		ToolCall: func(context.Context, string, map[string]any) (json.RawMessage, error) {
			return nil, errors.New("呼べません")
		},
	}
	src := `try { toolcall("x") } catch (e) { remind("失敗: " + e.message) }`
	if err := Run(context.Background(), src, funcs, time.Second); err != nil {
		t.Fatal(err)
	}
	if len(reminded) != 1 || !strings.Contains(reminded[0], "呼べません") {
		t.Fatalf("remind = %v", reminded)
	}
}

func TestRunErrors(t *testing.T) {
	for name, src := range map[string]string{
		"構文エラー":    "remind(",
		"未捕捉の例外":   `throw new Error("x")`,
		"空のremind": `remind("  ")`,
	} {
		if err := Run(context.Background(), src, Funcs{Remind: func(context.Context, string) error { return nil }}, time.Second); err == nil {
			t.Errorf("%s: エラーになりませんでした", name)
		}
	}
}

func TestRunTimeout(t *testing.T) {
	err := Run(context.Background(), "for(;;){}", Funcs{}, 50*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "終わりませんでした") {
		t.Fatalf("err = %v", err)
	}
}

func TestCompile(t *testing.T) {
	if err := Compile(`remind("a")`); err != nil {
		t.Fatal(err)
	}
	if err := Compile("remind("); err == nil {
		t.Fatal("構文エラーを検出できませんでした")
	}
}
