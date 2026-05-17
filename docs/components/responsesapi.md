# responsesapi component

## 1. 役割
- `responsesapi` は OpenAI Responses API との接続境界です。
- この component の責務は、内部の `ResponsesRequest` / `ToolResponse` を OpenAI API 呼び出しへ変換し、その結果を `ResponsesStreamChunk` / `ResponsesResponse` / `ToolRequest` として下流へ流すことです。
- 会話契約の妥当性検証、再試行条件、tool の実行自体はこの component の責務ではありません。

## 2. 構成
- `internal/components/responsesapi/client.go`
  - `/v1/responses` への HTTP リクエストを組み立てます。
  - 通常の streaming 応答取得、非 streaming 応答取得、tool result 再投入を担当します。
- `internal/components/responsesapi/runner.go`
  - stage として upstream event を受け取り、`client` 呼び出しと downstream event への変換を担当します。
  - tool call ごとに `ToolCallID -> ResponseID / RequestID` の対応を保持します。
- `internal/components/responsesapi/client_test.go`
  - stream の改行単位復元、stream 完了時の flush、completed response からの tool call 抽出を検証しています。

## 3. 主要フロー

### 3.1 通常の Responses API 呼び出し
1. `runner.handleRequest` が `types.ResponsesRequest` を受け取ります。
2. `Messages` が空なら `Role` と `Text` から 1 件の `ChatMessage` を組み立てます。
3. system prompt は stage 初期化時の `Config.Instructions` を基本とし、`ResponsesRequest.SystemPrompt` があればそれで上書きします。
4. `appendCurrentTimestamp` が system prompt の末尾に現在日付と現在時刻を追記します。
5. `client.CreateResponseStream` が `POST https://api.openai.com/v1/responses` を `stream=true` 付きで呼びます。
6. stream 中の各 NDJSON 1 行は `types.ResponsesStreamChunk{RequestID, Line}` として downstream に流します。
7. stream 完了後、tool call がなければ `types.ResponsesStreamChunk{RequestID, ResponseID, Done:true}` を流します。
8. tool call があれば完了 chunk は流さず、`handleResponsesResponse` に進みます。

### 3.2 stream 処理
1. `client.readResponseStream` が SSE を 1 行ずつ読みます。
2. `data:` 行だけを対象に JSON decode し、`handleStreamEvent` で event type ごとに処理します。
3. `response.output_text.delta` は `appendStreamDelta` に渡されます。
4. `appendStreamDelta` は delta をバッファへ連結し、改行 `\n` ごとに 1 行を切り出します。
5. 改行で確定した 1 行は `onLine` callback 経由で `runner` に渡されます。
6. stream 終了時に改行なしの残バッファがあれば、trim 後に 1 行として flush します。
7. `response.completed` は response 本体を保持し、`ResponseID` と tool call 抽出に使います。
8. `response.failed` は error として扱い、この component からは `ResponsesStreamChunk.Err` を持つ event として返されます。

### 3.3 tool call 抽出
1. tool call の抽出元は streaming 中の delta ではなく、`response.completed.response.output[]` です。
2. `client.extractToolCalls` は `output[]` を走査し、`type == "function_call"` の要素だけを対象にします。
3. `parseFunctionCall` は `name`、`call_id`、`arguments` を `types.ToolRequest` に変換します。
4. `arguments` は文字列でも object でも受け付け、空なら `{}` に補正します。
5. `call_id` が空の function call は無視します。
6. `runner.handleResponsesResponse` は `ResponsesResponse` を downstream に流した上で、各 tool call を `EventToolRequest` として流します。
7. その際、後続の再投入に必要な `ToolCallID -> ResponseID / RequestID` を内部 map に保存します。

### 3.4 tool result 再投入
1. `runner.handleToolResponse` が `types.ToolResponse` を受け取ります。
2. 保存していた `ToolCallID` 対応表から元の `ResponseID` と `RequestID` を取得します。
3. `client.SubmitToolOutput` が `previous_response_id` と `function_call_output` を含む payload を `/v1/responses` へ再送します。
4. 再投入 payload の `input` は 1 件の `{"type":"function_call_output","call_id":"...","output":"..."}` です。
5. 返ってきた follow-up 応答は `types.ResponsesResponse` に変換され、元の `RequestID` を付け直した上で `handleResponsesResponse` に渡されます。
6. follow-up が text なら `EventResponsesResponse` として流れ、さらに tool call を含むなら再度 `EventToolRequest` が発行されます。

## 4. 主要な入出力

### 4.1 入力
- `types.ResponsesRequest`
  - `Messages`: OpenAI に渡す会話履歴です。
  - `Text` / `Role`: `Messages` が空のときの簡易入力です。
  - `RequestID`: downstream へ戻す相関 ID です。
  - `SystemPrompt`: stage 既定値を上書きする任意 prompt です。
  - `ToolChoice`: 強制 tool 指定などをそのまま OpenAI payload に渡します。
  - `Tools`: request 単位の tool 定義です。`nil` のときは client のデフォルト定義を使います。
- `types.ToolResponse`
  - `ToolCallID`: どの function call への結果かを識別します。
  - `Output`: OpenAI に再投入する tool 実行結果です。

### 4.2 OpenAI API への主な payload
- 通常応答
```json
{
  "model": "...",
  "input": [
    {"role": "system", "content": "..."},
    {"role": "user", "content": "..."}
  ],
  "stream": true,
  "tools": [...],
  "tool_choice": {"type": "function", "name": "..."}
}
```
- tool result 再投入
```json
{
  "model": "...",
  "previous_response_id": "resp_...",
  "input": [
    {
      "type": "function_call_output",
      "call_id": "call_...",
      "output": "..."
    }
  ],
  "tools": [...]
}
```

### 4.3 出力
- `types.ResponsesStreamChunk`
  - `Line`: stream から復元した 1 行分のテキストです。
  - `Done`: stream 完了通知です。
  - `Err`: OpenAI 側の失敗、HTTP エラー、stream decode エラーなどを文字列で返します。
  - `ResponseID`: 完了時に取得できた response ID です。
- `types.ResponsesResponse`
  - `Text`: 非 streaming で取り出した応答本文です。
  - `ResponseID`: OpenAI response の ID です。
  - `RequestID`: 元の内部 request との相関 ID です。
  - `ToolCalls`: completed response から抽出した function call 一覧です。
- `types.ToolRequest`
  - `ResponseID`: function call を含んでいた OpenAI response の ID です。
  - `ToolCallID`: tool result 再投入時に再利用する call ID です。
  - `Name`: function 名です。
  - `Arguments`: tool 実行に渡す JSON 引数です。

## 5. 設計上の注意
- この component は通常会話を streaming で処理しますが、tool result 後の follow-up は `SubmitToolOutput` による非 streaming 応答です。
- `CreateResponse` は実装されていますが、`runner.handleRequest` の通常経路では使われていません。
- tool result 再投入時の `toolsOverride` は `nil` 固定で渡されています。そのため follow-up では client 初期化時のデフォルト tool 定義が使われます。
- `appendOutputConstraint` は現状 `Output` の trim のみを行い、追加の制約は加えていません。
- `response.output_text.delta` から復元した 1 行の意味づけや妥当性判定は、この component ではなく下流側に委ねられています。
- stream 中の function call delta を使う設計かどうかは、現行コードからは採用していないことだけが確認できます。将来方針は不明です。

## 6. 不明点
- tool result 再投入を streaming にしない理由は、現行コードと旧ドキュメントからは明示されておらず不明です。
- `ResponsesRequest.Tools` と client デフォルト tool の使い分け方針の全体設計は、この component 単体の実装だけでは不明です。

## 7. 参照元
- `internal/components/responsesapi/client.go`
- `internal/components/responsesapi/runner.go`
- `internal/components/responsesapi/client_test.go`
- `internal/types/types.go`
- `internal/types/event.go`
- `git show HEAD^:docs/3.LLM連携と出力契約.md`
- OpenAI Responses API Reference: https://platform.openai.com/docs/api-reference/responses?api-mode=responses
- OpenAI Function Calling Guide: https://platform.openai.com/docs/guides/function-calling?api-mode=responses
