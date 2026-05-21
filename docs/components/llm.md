# llm component

旧 `responsesapi` component は削除済みです。
現在の OpenAI Responses API 呼び出しは `internal/components/llm` が担当します。

## 役割

- 会話履歴Storeの `Snapshot()` を読み、LLM request を作る。
- OpenAI Responses API を streaming で呼ぶ。
- 出力を NDJSON 行として受け取る。
- `speech` / `wait` / `tool` の timeline item に変換する。
- 契約違反時は最大5回 retry する。
- 5回失敗した場合はログに出して応答を捨てる。

## NDJSON 契約

- `{"type":"speech","text":"..."}` は assistant 発話を表す。
- `{"type":"wait","sec":0.5}` は timeline 上の待機を表す。
- `{"type":"tool","name":"...","args":{...}}` は local tool 呼び出しを表す。
- `tool` は1回の LLM 応答の末尾に最大1件だけ許可する。
- OpenAI function calling の `tools`、`tool_choice`、`function_call_output` は使わない。

