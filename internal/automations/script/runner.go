// Package script は予約ジョブに登録されたJavaScriptを実行する。
// スクリプトからは remind（AIに話しかける）と toolcall（AIを通さずにツールを呼ぶ）だけを使える。
package script

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dop251/goja"
)

// DefaultTimeout はスクリプト1回の実行時間の上限。
const DefaultTimeout = 30 * time.Second

// Funcs はスクリプトから呼ばれる関数の実体。
type Funcs struct {
	// Remind は文字列をRealtime APIへ渡して応答させる。
	Remind func(ctx context.Context, text string) error
	// ToolCall はツールを直接実行し、結果のJSONを返す。
	ToolCall func(ctx context.Context, name string, args map[string]any) (json.RawMessage, error)
}

// Compile は構文を確かめる。登録時に呼び、誤りをAIへ返すために使う。
func Compile(src string) error {
	_, err := goja.Compile("job.js", src, true)
	return err
}

// Run はスクリプトを最後まで実行する。timeout が0以下なら DefaultTimeout を使う。
func Run(ctx context.Context, src string, funcs Funcs, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	program, err := goja.Compile("job.js", src, true)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	vm := goja.New()
	vm.SetFieldNameMapper(goja.TagFieldNameMapper("json", true))
	// 関数内でのエラーは、JSの例外として投げてスクリプト側でも捕まえられるようにする。
	throw := func(err error) { panic(vm.NewGoError(err)) }
	if err = vm.Set("remind", func(text string) {
		text = strings.TrimSpace(text)
		if text == "" {
			throw(errors.New("remind には伝える内容を渡してください"))
		}
		if funcs.Remind == nil {
			throw(errors.New("remind は使えません"))
		}
		if err := funcs.Remind(ctx, text); err != nil {
			throw(err)
		}
	}); err != nil {
		return err
	}
	if err = vm.Set("toolcall", func(name string, args map[string]any) any {
		name = strings.TrimSpace(name)
		if name == "" {
			throw(errors.New("toolcall にはツール名を渡してください"))
		}
		if funcs.ToolCall == nil {
			throw(errors.New("toolcall は使えません"))
		}
		if args == nil {
			args = map[string]any{}
		}
		output, err := funcs.ToolCall(ctx, name, args)
		if err != nil {
			throw(err)
		}
		var result any
		if err := json.Unmarshal(output, &result); err != nil {
			throw(fmt.Errorf("ツール結果の読み取り: %w", err))
		}
		return result
	}); err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, func() { vm.Interrupt("timeout") })
	defer stop()
	if _, err = vm.RunProgram(program); err != nil {
		var interrupted *goja.InterruptedError
		if errors.As(err, &interrupted) {
			return fmt.Errorf("スクリプトの実行が%v以内に終わりませんでした", timeout)
		}
		return err
	}
	return nil
}
