# responsesapi component

## 役割
- OpenAI Responses API との HTTP 境界を担当する。
- `EventResponsesRequest` を受け、`/v1/responses` を `stream=true` で呼び出す。
- LLM から返る NDJSON テキストを改行単位に復元し、`EventResponsesStreamChunk` として downstream に流す。
- NDJSON の意味解釈、契約検証、tool 実行は担当しない。

## 入力
| EventKind | payload | 用途 |
| --- | --- | --- |
| `EventResponsesRequest` | `types.ResponsesRequest` | `Messages` と system prompt から Responses API request を作る。 |

## 出力
| EventKind | payload | 用途 |
| --- | --- | --- |
| `EventResponsesStreamChunk` | `types.ResponsesStreamChunk` | stream から復元した NDJSON 1 行、完了通知、または error を返す。 |

## フロー
1. `runner.handleRequest` が `types.ResponsesRequest` を受け取る。
2. `Messages` が空なら `Role` / `Text` から 1 件の `ChatMessage` を組み立てる。
3. system prompt には現在日付・時刻を追記する。
4. `client.CreateResponseStream` が Responses API を `stream=true` で呼ぶ。
5. `response.output_text.delta` を内部バッファに連結し、改行ごとに `ResponsesStreamChunk.Line` として流す。
6. stream 終了時に残バッファがあれば 1 行として flush する。
7. `response.completed` から取得できた response id を `Done=true` chunk に載せる。
8. HTTP / SSE / API error は `ResponsesStreamChunk.Err` として返す。

## Tool について
- OpenAI API の tools / function calling 機能は使わない。
- tool call は LLM が通常テキストとして出力する NDJSON `{"type":"tool","name":"...","args":{...}}` を、下流の `conversation` が解釈する。
- tool result の再投入も `responsesapi` では行わない。tool runtime が会話コミット API に結果を渡し、`conversation` が次の `EventResponsesRequest` を作る。

## 参照元
- `internal/components/responsesapi/client.go`
- `internal/components/responsesapi/runner.go`
- `internal/components/responsesapi/client_test.go`
- `internal/types/types.go`
- `internal/types/event.go`
- OpenAI Responses API Reference: https://platform.openai.com/docs/api-reference/responses?api-mode=responses
