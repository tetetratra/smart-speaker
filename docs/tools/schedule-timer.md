# schedule_timer tool

## 概要
`schedule_timer` は、指定した秒数の経過後に内部 event を発火する function tool です。tool 自身は会話文面を直接生成せず、発火時の通知文は `EventTimerFired` の payload として downstream に渡され、その後の表示は `conversation` component 側で処理されます。

## 引数
`Definition()` で公開されている引数は次の 2 つです。

| 引数 | 型 | 必須 | 現在の扱い |
| --- | --- | --- | --- |
| `reminder_text` | `string` | 必須 | 空文字は不可です。発火時には `タイマーが発火しました: {reminder_text}` の形式で通知文へ埋め込まれます。 |
| `seconds` | `integer` | 必須 | 正の整数のみ受け付けます。0 以下はエラーです。 |

### バリデーション
- `reminder_text` が空文字の場合は `reminder_text is required` を返します。
- `seconds` が整数として解釈できない場合は `seconds must be an integer` を返します。
- `seconds` が 0 以下の場合は `seconds must be a positive integer` を返します。

### 補足
- `seconds` は `Run(args map[string]any)` 内で `float64`, `int`, `int32`, `int64` を受け付けます。
- 絶対時刻を直接指定する引数は現状ありません。

## 返り値
成功時の返り値は `map[string]any` で、現在は次の 2 項目だけを返します。

| キー | 型 | 内容 |
| --- | --- | --- |
| `scheduled_for` | `string` | 現在時刻に `seconds` を加えた発火予定時刻です。`time.RFC3339` 形式で返されます。 |
| `seconds` | `integer` | 入力で受け取った秒数です。 |

失敗時は tool result を返さず、`Run` の error として返します。

## 発火イベント
指定時間の経過後、`schedule_timer` は goroutine 上で `types.Event` を 1 件 emit します。

| 項目 | 値 |
| --- | --- |
| `Event.Kind` | `types.EventTimerFired` |
| `Event.Payload` | `types.TimerFiredEvent` |
| `TimerFiredEvent.ReminderText` | `タイマーが発火しました: {reminder_text}` |

### 発火しない条件
- `seconds <= 0` の場合はそもそも scheduling に進みません。
- `schedule()` 実行時点で待機時間が 0 以下なら即 return し、event は発火しません。
- 注入された `context.Context` が待機中に `Done()` になった場合、event は発火しません。

## conversation への通知
`conversation` component は `EventTimerFired` を `timerFiredSignal` に変換し、`timerFiredRule` で `EventRealtimeOutput` へ変換します。現在の通知内容は次のとおりです。

| 項目 | 値 |
| --- | --- |
| 出力 event | `types.EventRealtimeOutput` |
| payload 型 | `types.OutputLine` |
| `Role` | `assistant` |
| `Text` | `strings.TrimSpace(ReminderText)` の結果 |
| `Source` | `timer` |
| `Final` | 明示設定なし |

### 現在の挙動
- `ReminderText` が空白除去後に空なら、`conversation` は通知 event を出しません。
- `schedule_timer` の成功時 tool result 自体は別経路で `EventToolResponse` として `conversation` に入り、`write_diary` 以外の通常 tool と同様に会話 snapshot へ `ツール実行結果(schedule_timer): ...` という形で反映されます。
- タイマー満了時の通知は tool result ではなく、独立した `EventTimerFired` 由来の `EventRealtimeOutput` です。

## 不明点
- 再設計後も `seconds` 指定を維持するか、絶対時刻指定を追加するかは不明です。
- タイマー ID、キャンセル API、複数タイマーの識別子を返す設計にするかは不明です。
- `EventRealtimeOutput` を受け取った先の UI 表示仕様や読み上げ仕様は、この担当範囲の参照元だけでは確定できません。

## 参照元
- [internal/tools/functions/timer/tool.go](/Users/kondo.daichi/p/smart-speaker/internal/tools/functions/timer/tool.go)
- [internal/components/conversation/rule_timer_fired.go](/Users/kondo.daichi/p/smart-speaker/internal/components/conversation/rule_timer_fired.go)
- [internal/components/conversation/signal.go](/Users/kondo.daichi/p/smart-speaker/internal/components/conversation/signal.go)
- [internal/components/conversation/rule_tool_response.go](/Users/kondo.daichi/p/smart-speaker/internal/components/conversation/rule_tool_response.go)
- [internal/types/event.go](/Users/kondo.daichi/p/smart-speaker/internal/types/event.go)
- [internal/types/types.go](/Users/kondo.daichi/p/smart-speaker/internal/types/types.go)
- `git show HEAD^:docs/9.タイマー・日記・自動リセット.md`
- `git show HEAD^:docs/6.ツール実行基盤.md`
