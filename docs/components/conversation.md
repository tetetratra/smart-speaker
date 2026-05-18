# conversation component

## 概要
`conversation` component は、会話進行の正本を持つ component です。`types.Event` を内部 `signal` に正規化し、rule 群で内部 state を更新し、必要な副作用を event・timer・Responses API request として外部へ出します。

## 責務
- 人の確定発話を会話履歴へ反映し、assistant 応答 request を開始する。
- assistant 応答を `speech` / `wait` の timeline として解釈し、順番に進行させる。
- 再生中 assistant 発話、待機 timer、進行中 request を中断または継続する。
- 会話履歴を `EventConversationSnapshotUpdated` として共有する。
- 会話活動を `EventConversationActivity` として共有する。
- diary / calendar を system context として request に付与する。
- NDJSON 契約違反の応答を検知し、条件付きで 1 回だけ retry する。

## 入力 event
| EventKind | payload | 用途 |
| --- | --- | --- |
| `EventTextInput` | `types.OutputLine` | 空白除去後の text を `humanTextSignal` に変換し、人の確定発話として扱う。 |
| `EventSpeechStart` | なし | 発話開始検知。現行実装ではこれだけでは割り込みしない。 |
| `EventResponsesResponse` | `types.ResponsesResponse` | non-streaming 応答を解釈する。tool call を含む場合は会話本文としては処理しない。 |
| `EventResponsesStreamChunk` | `types.ResponsesStreamChunk` | streaming 応答の 1 行、完了、エラーを解釈する。 |
| `EventToolResponse` | `types.ToolResponse` | `write_diary` 以外の tool 実行結果を会話履歴へ反映する。 |
| `EventTTSEnd` | `types.TTSEvent` | 再生完了を受け、次の timeline 進行可否を判定する。 |
| `EventSessionClear` | なし | 会話 state を初期化する。 |

## 出力 event
| EventKind | payload | 出力条件 |
| --- | --- | --- |
| `EventResponsesRequest` | `types.ResponsesRequest` | 人の確定発話時、または invalid response retry 時。 |
| `EventRealtimeOutput` | `types.OutputLine` | assistant 発話開始時。text 行と `Final=true` 行を連続で出す。 |
| `EventTTSCancel` | `types.TTSCancel` | 再生中 assistant 発話を人の確定発話や session clear で中断するとき。 |
| `EventConversationSnapshotUpdated` | `types.ConversationSnapshot` | 人の確定発話時、tool 結果反映時、TTS 完了時、stream 完了時、session clear 時。 |
| `EventConversationActivity` | `types.ConversationActivity` | 人の確定発話時と assistant 発話開始時。 |

## 内部 state
| 項目 | 内容 |
| --- | --- |
| `conversation []*Utterance` | 会話履歴。`human` / `ai` / `tool` を保持する。 |
| `current *Utterance` | 現在再生中の assistant 発話。 |
| `utteranceByResponseID` | `EventTTSEnd` を対応付けるための index。 |
| `pendingTimeline []timelineSegment` | 未消化の `speech` / `wait` 列。 |
| `pendingTimelineIdx` | 次に消化する timeline の位置。 |
| `pendingRequestID` | 現在有効な Responses API request の ID。 |
| `pendingRequestCancelled` | 進行中 request を無効化したかどうか。 |
| `invalidResponseRetries` | 契約違反 retry の回数。最大 1。 |
| `pendingRequestStreaming` | streaming 応答を処理中かどうか。 |
| `pendingStreamSpeechStarted` | その stream で speech を 1 度でも開始したかどうか。 |
| `pendingStreamFailed` | stream を失敗扱いにした後の追加 chunk を無視するための flag。 |
| `pendingTimelineTimerWaiting` | wait 区間の timer 待ち中かどうか。 |
| `pendingStreamLines []string` | retry 用に保持する streaming 生行。 |
| `seq` | `human_*` / `ai_*` / `resp_*` / `req_*` などの採番に使う連番。 |

## rule 群
| rule | 責務 |
| --- | --- |
| `speechStartRule` | `EventSpeechStart` を処理済みにするが、中断はしない。 |
| `humanTextRule` | 会話中断、user utterance 追加、snapshot/activity emit、Responses request 生成。 |
| `responsesRule` | non-streaming 応答の NDJSON 契約検証、timeline 反映、retry 判定。 |
| `responsesStreamRule` | streaming chunk の逐次検証、timeline 追加、即時進行、stream 完了処理。 |
| `toolResponseRule` | `write_diary` を除く tool 結果を `SpeakerTool` として履歴へ追加。 |
| `sessionClearRule` | 進行中会話を止めて state を初期化し、空 snapshot を出す。 |
| `ttsEndRule` | 再生完了を反映し、wait 消費と次発話への進行を決める。 |
| `timerElapsedRule` | wait timer 満了後に timeline を再開する。 |

rule の適用順は `speechStartRule` → `humanTextRule` → `responsesRule` → `responsesStreamRule` → `toolResponseRule` → `sessionClearRule` → `ttsEndRule` → `timerElapsedRule` です。

## timeline 進行
1. 人の確定発話を受けると、現在の再生・wait timer・pending request・未再生 utterance を中断し、user message を履歴へ追加する。
2. `buildConversationMessages()` で会話履歴を `types.ChatMessage` に変換し、diary / calendar context を先頭へ付与して `EventResponsesRequest` を出す。
3. 応答は `{"type":"speech","text":"..."}` または `{"type":"wait","sec":整数}` の NDJSON として解釈する。
4. `responsesRule` は応答全体をまとめて `pendingTimeline` に積む。`responsesStreamRule` は行単位で `pendingTimeline` に追記する。
5. `advanceTimelineEffects()` は `pendingTimelineIdx` から順に segment を消化する。
6. `wait` は秒数を 0..5 に正規化し、0 より大きければ内部 timer を開始する。0 の場合は次 segment へ進む。
7. `speech` は `Utterance` を作って `EventRealtimeOutput` を出し、TTS 完了待ちに入る。
8. `EventTTSEnd` を受けると対象 utterance を `Played` にし、後続に speech があれば先頭の wait 群をまとめて消費して timer を張る。
9. streaming 中は、speech を 1 度も開始していない段階の契約違反だけ retry 対象になる。speech 開始後の invalid chunk は retry せず、その stream の残り進行を破棄する。
10. stream 完了時に speech が 1 つも無ければ invalid response として retry する。speech 済みで後続進行が無ければ snapshot を更新する。

## ルール上の補足
- assistant 発話は `EventRealtimeOutput` を出した時点で利用者へ提示済みとみなし、`EventTTSEnd` 前でも次 request の会話履歴に含める。
- `toolResponseRule` は tool 出力を `system` role の履歴として保存する。
- `write_diary` の tool 結果は `conversation` の履歴へ追加しない。
- `EventSpeechEnd`、`EventRTCVADStatus`、`EventRealtimeAudio`、`EventRTCSignal` はこの component では処理していない。

## 不明点
- `conversation` component 単体の責務として、`EventResponsesResponse` に含まれる `ToolCalls` の downstream 実行フロー全体は定義されていません。実行自体は `responsesapi` / `toolcaller` 側の責務です。
- LLM へ渡す system prompt 本文そのものは、この component 配下の実装だけでは不明です。

## 参照元
- `internal/components/conversation/conversation.go`
- `internal/components/conversation/core.go`
- `internal/components/conversation/state.go`
- `internal/components/conversation/signal.go`
- `internal/components/conversation/rule_*.go`
- `internal/components/conversation/runtime_*.go`
- `internal/components/conversation/response_contract.go`
- `internal/components/conversation/context_provider.go`
- `internal/components/conversation/context_calendar_format.go`
- `internal/components/conversation/conversation_integration_test.go`
- `internal/components/conversation/response_contract_test.go`
- `internal/types/event.go`
- `internal/types/types.go`
- `git show HEAD^:docs/2.会話オーケストレーション.md`
- `git show HEAD^:docs/3.LLM連携と出力契約.md`
