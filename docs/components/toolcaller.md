# toolcaller component

## 役割
- `EventToolRequest` を受け取り、対応する local tool handler を非同期に実行する。
- 実行結果は downstream event ではなく、`conversation.ToolResultSink` へ commit する。
- tool 内部で発生する副作用 event、たとえば `EventWhiteboardUpdate` は downstream に流す。

## 入力
| EventKind | payload | 用途 |
| --- | --- | --- |
| `EventToolRequest` | `types.ToolRequest` | `Name` で handler を選び、`Arguments` を JSON decode して実行する。 |

## 出力
| 経路 | 内容 |
| --- | --- |
| `ToolResultSink.Commit` | `types.ToolResponse` を conversation に戻す。 |
| downstream | `EventEmitterAware` tool が発火した内部 event を流す。 |

## フロー
1. stage 起動時に handler へ `context.Context` と event emitter を注入する。
2. `EventToolRequest` を受け取ると request ごとに goroutine を起動する。
3. `Arguments` を `map[string]any` に decode する。失敗時は空 map として扱う。
4. handler が見つからない場合は `{"error":"unknown tool: ..."}` 相当の result を作る。
5. handler error も panic させず `{"error":"..."}` に正規化する。
6. result を JSON に encode し、元 request の `ToolCallID` / `Name` / `GenerationID` を付けて `ToolResultSink` へ commit する。

## 境界
- LLM に提示する tool 定義は `registry.Definitions()` を system prompt に埋め込む。
- `toolcaller` は順序制御をしない。speech / wait / tool の順序は `conversation` の timeline と scheduler が決める。
- UI には tool call / result を表示しない。
- tool result は古い世代のものでも conversation に戻す。現在の話題に使うか無視するかは、履歴に含めた stale 情報を見て LLM が判断する。

## 参照元
- `internal/components/toolcaller/toolcaller.go`
- `internal/components/conversation/tool_result_sink.go`
- `internal/tools/interfaces.go`
- `internal/tools/registry/registry.go`
- `internal/types/event.go`
