# conversation component

## 役割
- 会話履歴、世代 id、応答 timeline の正本を持つ。
- `EventHumanUtterance` と tool result を LLM 入力として commit し、`EventResponsesRequest` を生成する。
- LLM stream の NDJSON 契約を検証し、`speech` / `wait` / `tool` を順番に実行する。
- UI 表示、TTS、tool 実行の発火タイミングを制御する。

## 入力
| 入力 | 経路 | 用途 |
| --- | --- | --- |
| `EventHumanUtterance` | graph | human 発話を履歴に保存し、新しい世代を開始する。 |
| `EventResponsesStreamChunk` | graph | LLM の NDJSON 1 行、完了、error を処理する。 |
| `EventTTSEnd` | graph | 現在の assistant 発話完了を受け、次の timeline item に進む。 |
| `types.ToolResponse` | `ToolResultSink` | tool result を履歴に保存し、LLM を再実行する。 |
| timer elapsed | internal | `wait` 終了後に次の timeline item に進む。 |

## 出力
| EventKind | payload | 用途 |
| --- | --- | --- |
| `EventResponsesRequest` | `types.ResponsesRequest` | LLM へ会話履歴を渡して推論を開始する。 |
| `EventRealtimeOutput` | `types.OutputLine` | assistant speech を UI / TTS に渡す。 |
| `EventToolRequest` | `types.ToolRequest` | timeline 上の順序が来た tool を実行させる。 |

## NDJSON 契約
LLM 応答は 1 行 1 JSON object の NDJSON です。

```json
{"type":"speech","text":"確認するね"}
{"type":"wait","sec":1}
{"type":"tool","name":"get_temp","args":{"room":"living"}}
```

- `speech.text` は空文字不可。
- `wait.sec` は必須で、実行時に 0〜5 秒へ丸める。
- `tool.name` は必須。`args` が省略された場合は `{}` 扱い。
- `tool` は 1 回の LLM 応答の末尾に最大 1 件だけ置ける。
- `tool` 後に別 chunk が来た場合は契約違反として扱う。

## 世代管理
- human 発話ごとに `generation` を単調増加させる。
- LLM request と tool request には現在世代 id を付ける。
- 古い世代の LLM chunk は破棄する。
- すでに `EventRealtimeOutput` として出した assistant speech は止めない。
- 新しい human 発話または tool result が来た場合、未再生の pending timeline と未完了 request は破棄する。
- tool result は古い世代でも破棄しない。履歴には `stale=true` と現在世代を含め、LLM に無視するか判断させる。

## Timeline
`pendingTimeline` は LLM から届いた順序付き item を保持します。

- `speech`: assistant `Utterance` を履歴に追加し、`EventRealtimeOutput` を text 行と final 行で出す。
- `wait`: timer を開始し、満了後に次へ進む。
- `tool`: `EventToolRequest` を出し、tool result は `ToolResultSink` から戻る。

`speech` の後続 item は `EventTTSEnd` を待ってから進みます。これにより「確認するね」などの発話が再生コンポーネントへ渡る前に tool が発火することを避けます。

## Rule
| Rule | トリガー | 主な処理 |
| --- | --- | --- |
| `humanTextRule` | `humanTextSignal` | 世代を進め、human 発話を保存し、LLM request を出す。 |
| `responsesStreamRule` | `responsesStreamChunkSignal` | stream chunk を検証し、timeline に追加する。 |
| `toolResponseRule` | `toolResponseSignal` | tool result を履歴に保存し、LLM request を出す。 |
| `ttsEndRule` | `ttsEndSignal` | speech 完了後に後続 `wait` / `speech` / `tool` へ進む。 |
| `timerElapsedRule` | `timerElapsedSignal` | `wait` 完了後に timeline を進める。 |

適用順は `humanTextRule` → `responsesStreamRule` → `toolResponseRule` → `ttsEndRule` → `timerElapsedRule` です。

## State
| 項目 | 内容 |
| --- | --- |
| `conversation []*Utterance` | human / ai / tool の会話履歴。 |
| `generation uint64` | 最新世代 id。 |
| `requestGeneration map[string]uint64` | request id と世代 id の対応。 |
| `current *Utterance` | TTS 完了待ちの assistant 発話。 |
| `pendingTimeline []timelineSegment` | 未処理の `speech` / `wait` / `tool`。 |
| `pendingRequestID` | 現在有効な LLM request id。 |
| `pendingStream*` | stream 契約検証と retry 用の一時状態。 |

## 参照元
- `internal/components/conversation/`
- `internal/types/event.go`
- `internal/types/types.go`
