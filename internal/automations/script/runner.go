// Package script は予約用の同期JavaScriptを実行する。
package script

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja/parser"
	"github.com/google/uuid"
	"github.com/tetetratra/smart-speaker/internal/tools/execution"
	"github.com/tetetratra/smart-speaker/internal/types"
)

const MaxDuration = 30 * time.Second

type Runner struct {
	execute func(context.Context, types.ToolRequest) types.ToolResultRecord
	deliver func(context.Context, types.InboundMessage) error
	timeout time.Duration
}

func New(execute func(context.Context, types.ToolRequest) types.ToolResultRecord, deliver func(context.Context, types.InboundMessage) error) *Runner {
	return &Runner{execute: execute, deliver: deliver, timeout: MaxDuration}
}

// Compile はファイルやURLからsource mapを読み込まずに構文を検証する。
func Compile(source string) (*goja.Program, error) {
	tree, err := parser.ParseFile(nil, "automation.js", source, 0, parser.WithDisableSourceMaps)
	if err != nil {
		return nil, err
	}
	return goja.CompileAST(tree, false)
}

// Run は非同期ツールの最終結果を待たず、同期部分だけを制限時間内に実行する。
func (r *Runner) Run(parent context.Context, id, name, source string) (runErr error) {
	runID := uuid.NewString()
	var deliveryErr error
	metadata := map[string]string{"delivery": "history_only", "automation_id": id, "run_id": runID}
	publish := func(ctx context.Context, suffix, role, text string, meta map[string]string) error {
		if r.deliver == nil {
			return fmt.Errorf("履歴の送信経路がありません")
		}
		raw, _ := json.Marshal(meta)
		err := r.deliver(ctx, types.InboundMessage{ID: runID + "/" + suffix, Role: role, Source: "automation/" + id, Text: text, Context: raw})
		if err != nil && deliveryErr == nil {
			deliveryErr = err
		}
		return err
	}
	if err := publish(parent, "start", types.RoleSystem, "[予約ジョブ開始: "+name+"]", metadata); err != nil {
		return err
	}
	defer func() {
		text := "[予約ジョブ終了: " + name + "] スクリプトの実行が終了しました。非同期処理は継続することがあります。"
		if runErr != nil {
			text = "[予約ジョブ失敗: " + name + "] " + runErr.Error()
		}
		if err := publish(parent, "end", types.RoleSystem, text, metadata); err != nil && runErr == nil {
			runErr = err
		}
	}()
	program, err := Compile(source)
	if err != nil {
		return fmt.Errorf("スクリプトの構文: %w", err)
	}
	ctx, cancel := context.WithTimeout(parent, r.timeout)
	defer cancel()
	vm := goja.New()
	vm.SetParserOptions(parser.WithDisableSourceMaps)
	// 予約は同期JSだけを扱う。待ち続けるPromiseは作らせない。
	_ = vm.Set("Promise", goja.Undefined())
	rejected := map[*goja.Promise]bool{}
	vm.SetPromiseRejectionTracker(func(p *goja.Promise, operation goja.PromiseRejectionOperation) {
		if operation == goja.PromiseRejectionReject {
			rejected[p] = true
		} else {
			delete(rejected, p)
		}
	})
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			vm.Interrupt(ctx.Err())
		case <-done:
		}
	}()
	check := func() {
		if deliveryErr != nil {
			panic(vm.NewGoError(deliveryErr))
		}
		if err := ctx.Err(); err != nil {
			panic(vm.NewGoError(err))
		}
	}
	_ = vm.Set("toolcall", func(call goja.FunctionCall) goja.Value {
		check()
		name, ok := call.Argument(0).Export().(string)
		if !ok || strings.TrimSpace(name) == "" {
			return vm.ToValue(map[string]any{"error": "ツール名を文字列で指定してください"})
		}
		var args map[string]any
		raw, err := json.Marshal(call.Argument(1).Export())
		if err != nil || json.Unmarshal(raw, &args) != nil || args == nil {
			return vm.ToValue(map[string]any{"error": "引数をJSONオブジェクトで指定してください"})
		}
		callID := uuid.NewString()
		meta := map[string]string{"delivery": "history_only", "automation_id": id, "run_id": runID, "call_id": callID, "tool_name": name}
		if err := publish(ctx, "call/"+callID, types.RoleToolCall, name+": "+string(raw), meta); err != nil {
			panic(vm.NewGoError(err))
		}
		deliveryContext, _ := json.Marshal(meta)
		req := types.ToolRequest{ToolCallID: callID, Name: name, Arguments: raw}
		result := types.ToolResultRecord{ToolCallID: callID, Name: name, Output: json.RawMessage(`{"error":"ツールの実行経路がありません"}`)}
		if r.execute != nil {
			result = r.execute(execution.WithDeliveryContext(ctx, deliveryContext), req)
		}
		var out map[string]any
		if json.Unmarshal(result.Output, &out) != nil || out == nil {
			out = map[string]any{"error": "ツールの戻り値をJSONとして解釈できません"}
		}
		// 画面とJSには元の結果を返し、会話履歴へは通知用の指示を持ち込まない。
		historyOut := make(map[string]any, len(out))
		for key, value := range out {
			if key != "instructions" {
				historyOut[key] = value
			}
		}
		historyRaw, _ := json.Marshal(historyOut)
		if err := publish(parent, "result/"+callID, types.RoleToolResult, name+": "+string(historyRaw), meta); err != nil {
			panic(vm.NewGoError(err))
		}
		check()
		return vm.ToValue(out)
	})
	_ = vm.Set("remind", func(call goja.FunctionCall) goja.Value {
		check()
		text, ok := call.Argument(0).Export().(string)
		if !ok || strings.TrimSpace(text) == "" {
			panic(vm.NewTypeError("remindの本文を文字列で指定してください"))
		}
		if r.deliver == nil {
			panic(vm.NewGoError(fmt.Errorf("通知の送信経路がありません")))
		}
		prompt := types.InboundMessage{ID: uuid.NewString(), Source: "automation/" + id, Text: "[アプリの予約ジョブ: " + name + "]\n予定時刻になりました。次の指示を今すぐ実行してください。指示に時刻や「〜後に」などの表現が含まれていても、それは予約時の表現なので、新しい予約の登録や予約内容の復唱はしないでください。\n指示: " + text}
		if err := r.deliver(ctx, prompt); err != nil {
			if deliveryErr == nil {
				deliveryErr = err
			}
			panic(vm.NewGoError(err))
		}
		return goja.Undefined()
	})
	value, err := vm.RunProgram(program)
	if deliveryErr != nil {
		return fmt.Errorf("履歴または通知の受付: %w", deliveryErr)
	}
	if ctx.Err() != nil {
		return fmt.Errorf("スクリプトの実行期限: %w", ctx.Err())
	}
	if err != nil {
		return fmt.Errorf("スクリプトの実行: %w", err)
	}
	if len(rejected) > 0 {
		return fmt.Errorf("Promiseの未処理の失敗があります。予約スクリプトは同期処理で記述してください")
	}
	if value != nil {
		if p, ok := value.Export().(*goja.Promise); ok && p.State() == goja.PromiseStatePending {
			return fmt.Errorf("未完了のPromiseは利用できません。予約スクリプトは同期処理で記述してください")
		}
	}
	return nil
}
