# web_search tool

## 概要
`web_search` は、最新情報や外部情報が必要な会話で使う local tool です。
LLM からは通常の JSON timeline tool item として呼び出され、handler 内部で OpenAI Responses API の hosted `web_search` を別 request として利用します。

OpenAI function calling は使いません。
会話 pipeline 上は他の local tool と同じく、`scheduler` / `router` / `toolcaller` を経由し、結果は `conversationcommitter` で会話履歴に保存されます。

## 登録条件
- 通常起動では `cmd/smart-speaker/main.go` の `buildToolRegistry` が `OPENAI_API_KEY` と `OPENAI_RESPONSES_MODEL` を registry に渡します。
- registry は OpenAI API key と model が両方ある場合に `web_search` を登録します。
- 登録されると `registry.Definitions()` 経由で LLM の Structured Outputs schema と system prompt に入り、`registry.Handlers()` 経由で `toolcaller` の handler map に入ります。
- API key または model が空の場合、`web_search` は LLM に提示されず handler も登録されません。

通常起動では `OPENAI_API_KEY` は必須設定です。
`OPENAI_RESPONSES_MODEL` が未設定の場合は `internal/app/config.go` の default model が使われます。

## 入力
- tool 名: `web_search`
- 必須引数: `query`
- 型: `string`
- `parameters.required` は `query` のみです。
- `additionalProperties` は `false` です。
- handler 実行時にも `query` 以外の引数は `unsupported argument: <name>` として拒否します。

```json
{
  "type": "object",
  "properties": {
    "query": {
      "type": "string",
      "description": "検索したい内容。"
    }
  },
  "required": ["query"],
  "additionalProperties": false
}
```

呼び出し例:

```json
{"type":"tool","name":"web_search","args":{"query":"OpenAI Responses API web_search 最新仕様"}}
```

## 出力
成功時の返り値は `result` のみです。

```json
{
  "result": "検索結果を踏まえた回答本文"
}
```

- `result` には OpenAI Responses API の回答本文を入れます。
- citation URL、annotations、sources などの補助情報は返しません。
- tool result の形を単純に保ち、後続の LLM が追加フィールドに引っ張られないようにします。

## 内部処理
1. LLM が JSON timeline の末尾 item として `web_search` を出力します。
2. scheduler / router が `ToolRequest` として `toolcaller` に渡します。
3. `internal/tools/functions/websearch.Tool` が `query` を検証します。
4. handler 専用 client が OpenAI Responses API `POST /v1/responses` を呼び出します。
5. request body には `tools: [{"type":"web_search"}]` を指定します。
6. response の `output_text` を優先して読み、空の場合は `output[].content[].text` の `output_text` を fallback として読みます。
7. 抽出した本文を `{"result":"..."}` として `toolcaller` に返します。
8. `toolcaller` が JSON 化して `ToolResultRecord.Output` に入れ、`conversationcommitter.ResultAPI.CommitToolResult` で会話履歴に保存します。

Responses API request の概形:

```json
{
  "model": "<OPENAI_RESPONSES_MODEL>",
  "input": "<query>",
  "tools": [
    {"type": "web_search"}
  ],
  "include": [
    "web_search_call.action.sources"
  ]
}
```

`include` は OpenAI 側の検索実行に必要な情報取得を許可するために指定していますが、tool result として LLM に返すのは回答本文だけです。

## エラーと制約
- `query` が空、または文字列でない場合は `query is required` を返します。
- `query` 以外の引数がある場合は `unsupported argument: <name>` を返します。
- API key が空の場合は `web_search: API key is required` を返します。
- model が空の場合は `web_search: model is required` を返します。
- Responses API が HTTP 300 以上を返した場合は status と response body を含む error にします。
- response に本文がない場合は `web_search: result is empty` を返します。
- 実 OpenAI API への通信は handler 実行時に発生します。unit test では差し替え client や test HTTP server で外部通信なしに検証します。

## 参照元
- `internal/tools/functions/websearch/tool.go`
- `internal/tools/functions/websearch/client.go`
- `internal/tools/registry/registry.go`
- `cmd/smart-speaker/main.go`
- `internal/components/llm/prompt_tools.go`
- `internal/components/toolcaller/toolcaller.go`
