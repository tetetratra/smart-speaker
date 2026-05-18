# toolcaller component

## 1. 役割
- `toolcaller` は tool 実行の境界です。
- この component の責務は、`types.ToolRequest` を受けて handler を解決し、実行結果を `types.ToolResponse` として返すことです。
- handler 定義の登録方針、OpenAI Responses API への再投入、WebSocket での表示変換はこの component の責務ではありません。

## 2. 担当範囲
- `registry` から受け取った `name -> handler` の保持
- `ToolRequest.Name` による handler 解決
- `ToolRequest.Arguments` の JSON decode
- `ContextAware` / `EventEmitterAware` への実行コンテキスト注入
- handler 実行
- 実行結果と実行エラーの `ToolResponse` 化
- tool 実行中に発火された内部 event の downstream 送出

## 3. 責務と境界
### toolcaller が担当すること
- stage 起動時に `context.Context` を生成し、対応する tool instance へ注入する。
- `EventToolRequest` だけを処理対象とし、他の event は無視する。
- request ごとに goroutine を分けて tool を非同期実行する。
- handler の戻り値 `map[string]any` を JSON にエンコードして `types.ToolResponse.Output` に格納する。
- 未知の tool 名、引数 decode 失敗、handler error、結果 encode 失敗を panic ではなく error payload に正規化する。

### toolcaller が担当しないこと
- どの tool を利用可能にするかの決定
- LLM に渡す tool 定義の生成
- `ToolResponse` を OpenAI へ再投入する処理
- `ToolRequest` / `ToolResponse` を UI 表示用 JSON に変換する処理
- tool ごとの業務ロジックそのもの

## 4. 主要構成
- `internal/components/toolcaller/toolcaller.go`
  - stage 本体です。
  - upstream / downstream channel、stage 全体の `context`、終了待ち用 `WaitGroup`、handler map を持ちます。
- `internal/tools/interfaces.go`
  - `tools.Handler`、`tools.ContextAware`、`tools.EventEmitterAware` の契約を定義します。
- `internal/tools/registry/registry.go`
  - `toolcaller` に渡す handler map の供給元です。
- `internal/types/event.go`
  - `types.ToolRequest`、`types.ToolResponse`、`EventToolRequest`、`EventToolResponse` を定義します。

## 5. データフロー
### 5.1 handler 解決から `ToolResponse` 返却まで
1. `toolcaller.run` が stage 用の `context.Context` を作成します。
2. 起動時に全 handler を走査し、`tools.ContextAware` 実装には `SetContext(ctx)` を呼びます。
3. 同じく `tools.EventEmitterAware` 実装には `SetEventEmitter(s.emit)` を呼びます。
4. upstream から `EventToolRequest` を受けると、payload を `types.ToolRequest` として取り出します。
5. `dispatchTool` が request ごとの goroutine を開始します。
6. `executeTool` が `ToolRequest.Arguments` を `map[string]any` に decode します。
7. `req.Name` で handler map を引きます。
8. handler が見つかれば `tool.Run(args)` を実行し、見つからなければ `{"error":"unknown function: ..."}` を組み立てます。
9. handler error は `{"error":"..."}` に変換します。
10. 結果 map を JSON encode し、`types.ToolResponse{ToolCallID, Name, ResponseID, Output}` を返します。
11. `dispatchTool` が `EventToolResponse` として downstream に流します。

### 5.2 tool 内部 event の送出
1. `EventEmitterAware` な tool は起動時に `emit` 関数を受け取ります。
2. tool 実行中にその関数を呼ぶと、`toolcaller` は `ctx.Done()` を尊重しつつ event を downstream に流します。
3. この event は `ToolResponse` とは別経路です。
4. たとえば `set_whiteboard` は `EventWhiteboardUpdate` を発火できます。

## 6. 入出力
### 6.1 入力
- `types.Event{Kind: types.EventToolRequest, Payload: types.ToolRequest}`
  - `ResponseID`: 元の Responses API 応答 ID です。
  - `ToolCallID`: tool call の相関 ID です。
  - `Name`: 実行対象 handler 名です。
  - `Arguments`: tool に渡す JSON 引数です。

### 6.2 出力
- `types.Event{Kind: types.EventToolResponse, Payload: types.ToolResponse}`
  - `ToolCallID`: 入力 request と同じ call ID です。
  - `Name`: 実行した tool 名です。
  - `ResponseID`: 入力 request と同じ response ID です。
  - `Output`: handler 実行結果または error payload の JSON です。
- `tools.EventEmitterAware` な tool が発火した任意の内部 event

## 7. registry との関係
- `toolcaller.NewStage` は `handlers map[string]tools.Handler` を受け取るだけで、登録ロジックは持ちません。
- 通常は `registry.Registry.Handlers()` が `toolcaller` への入力になります。
- `registry` 側は `entry.handler != nil` のものだけを `name -> handler` に変換します。
- `web_search` のように definition だけを持ち local handler を持たない tool は、`toolcaller` の実行対象には入りません。
- `registry.New` は handler instance を 1 回生成し、その instance を `toolcaller` に渡します。`toolcaller` はその instance に対して起動時に context と emitter を注入します。

## 8. 周辺 component との関係
- `responsesapi`
  - `EventToolRequest` の主な発行元です。
  - `toolcaller` が返した `EventToolResponse` を受けて、`function_call_output` として OpenAI Responses API に再投入します。
- `wschat`
  - `EventToolRequest` を `function_call` としてクライアントへ流します。
  - `EventToolResponse` を `function_result` としてクライアントへ流します。
  - `toolcaller` から emit された `EventWhiteboardUpdate` もここで `whiteboard_update` に変換されます。
- `conversation`
  - `EventToolResponse` を会話履歴へ反映します。

## 9. エラー処理と終了処理
- `ToolRequest` payload の型が想定外なら log を出してその request を無視します。
- `Arguments` の JSON decode に失敗した場合は log を出し、空 map で handler を呼びます。
- 未登録 tool 名は `{"error":"unknown function: <name>"}` を返します。
- handler が error を返した場合は `{"error":"<message>"}` を返します。
- 結果の JSON encode に失敗した場合は `{"error":"result encoding failed"}` を返します。
- `close()` は cancel 後に進行中 task 完了を待ち、最後に downstream を閉じます。

## 10. 現状の前提と制約
- handler 解決は単純な文字列一致です。alias や fallback 解決はありません。
- argument schema の検証は `toolcaller` では行いません。decode 後の解釈は各 handler 側に委ねられます。
- `ContextAware` / `EventEmitterAware` の注入は request ごとではなく stage 起動時に 1 回です。
- handler instance は `registry` から共有されるため、stateful な tool が安全かどうかは各 tool 実装に依存します。
- 実行順序保証はありません。複数 request は goroutine で並行実行されます。

## 11. 不明点
- handler instance を複数 stage 間で共有する前提かどうかは、参照実装だけでは不明です。
- tool 実行数の上限や backpressure 方針は、現行実装と旧ドキュメントからは不明です。

## 12. 参照元
- `internal/components/toolcaller/toolcaller.go`
- `internal/tools/interfaces.go`
- `internal/tools/registry/registry.go`
- `internal/tools/registry/registry_test.go`
- `internal/components/responsesapi/runner.go`
- `internal/components/wschat/wschat.go`
- `internal/components/conversation/rule_tool_response.go`
- `internal/types/event.go`
- `git show HEAD^:docs/6.ツール実行基盤.md`
