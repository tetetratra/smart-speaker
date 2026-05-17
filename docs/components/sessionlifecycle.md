# sessionlifecycle component

## 概要
`sessionlifecycle` component は、会話の無操作時間を監視し、一定時間を過ぎた会話を `write_diary` によって要約してから `EventSessionClear` で閉じる component です。会話内容そのものを解釈するのではなく、`conversation` から共有される snapshot と activity を使って、日記化と session clear の順序だけを制御します。

## 責務
- `EventConversationSnapshotUpdated` を受け取り、最新の会話 snapshot を保持する。
- `EventConversationActivity` を受け取り、最終活動時刻を更新し、idle timer を張り直す。
- idle timeout 到達時に、`write_diary` 専用の `EventResponsesRequest` を出す。
- `write_diary` の `EventToolResponse` を受けたら内部 state をリセットし、`EventSessionClear` を出す。

## 入力 event
| EventKind | payload | 用途 |
| --- | --- | --- |
| `EventConversationSnapshotUpdated` | `types.ConversationSnapshot` | 日記化対象として使う会話 snapshot を更新する。 |
| `EventConversationActivity` | `types.ConversationActivity` | 最終活動時刻を更新し、idle timer を再設定する。 |
| `EventToolResponse` | `types.ToolResponse` | `write_diary` 完了だけを監視し、session clear の発火条件に使う。 |

## 出力 event
| EventKind | payload | 出力条件 |
| --- | --- | --- |
| `EventResponsesRequest` | `types.ResponsesRequest` | idle timeout 到達後、日記化条件を満たすとき。 |
| `EventSessionClear` | なし | `write_diary` の完了通知を受け取り、かつその日記化要求がまだ有効なとき。 |

## 内部 state
| 項目 | 内容 |
| --- | --- |
| `lastActivityAt` | 最後に観測した会話活動時刻。 |
| `snapshot` | 直近の会話 snapshot。日記化 request にそのまま複製して使う。 |
| `diaryInFlight` | `write_diary` 要求を出した後、完了待ち中かどうか。 |
| `timer` / `timerC` | idle timeout を監視する timer。 |

## idle timeout
`IdleThreshold` が 0 以下なら既定値の 60 分を使います。`EventConversationActivity` を受けるたびに `lastActivityAt` を更新し、既存 timer を止めてから `lastActivityAt + IdleThreshold` までの残り時間で timer を張り直します。

timeout 到達時も、すぐに日記化へ進むとは限りません。以下のいずれかに当てはまる場合は何も出力しません。

- すでに `diaryInFlight` が `true`。
- `snapshot` が空。
- `WriteDiaryTools` が空。
- `lastActivityAt` が未設定。

また、timeout 時点でまだ `lastActivityAt + IdleThreshold` に達していなければ、残り時間で timer を再設定します。

## write_diary 要求
idle timeout を満たすと、`sessionlifecycle` は `diaryInFlight = true` にしたうえで `EventResponsesRequest` を出します。request の構成は次のとおりです。

- `Role`: `system`
- `Text`: 固定の日記化 prompt に、会話量に応じた文量指示を加えた文字列
- `Messages`: 保持している `snapshot` の複製
- `SystemPrompt`: 空文字へのポインタ
- `ToolChoice`: `{"type":"function","name":"write_diary"}`
- `Tools`: `Config.WriteDiaryTools`

文量指示は `user` / `assistant` の非空 message 数だけを数えて決まります。

| 会話数 | 指示 |
| --- | --- |
| 1〜6 | 1行程度 |
| 7〜12 | 2〜3行程度 |
| 13 以上 | 3〜5行程度 |

`system` message や空文字 message は、この判定には含まれません。

## session clear
`EventToolResponse` の `Name` が `write_diary` 以外なら無視します。`write_diary` であっても `diaryInFlight` が `false` なら無視します。これは、日記化中に新しい会話活動が発生した場合、古い `write_diary` 完了通知で誤って session を閉じないためです。

有効な `write_diary` 完了を受けた場合は、次の順で処理します。

1. `diaryInFlight` を `false` に戻す。
2. timer を停止する。
3. `state` 全体を初期化する。
4. `EventSessionClear` を出す。

## conversation との関係
`sessionlifecycle` は `conversation` の正本 state を持ちません。`conversation` が出す event を受けて追従し、会話終了の orchestration だけを担当します。

### `conversation` から受け取るもの
- `EventConversationSnapshotUpdated`
  - `conversation` が現在の会話履歴を snapshot として共有する。
- `EventConversationActivity`
  - 人の確定発話時に `Source=human_turn_committed`、assistant 発話開始時に `Source=assistant_turn_started` が流れる。

### `conversation` へ返すもの
- `EventSessionClear`
  - `conversation` は進行中の会話を中断し、内部会話 state を初期化し、空の snapshot を再送する。

### diary との境界
- `sessionlifecycle` 自体は diary を永続化しない。
- diary への書き込みは `write_diary` tool の責務。
- 次回会話で diary を system context として注入するのは `conversation` の `contextProvider` の責務。
- `conversation` は `write_diary` の `ToolResponse` を通常の会話履歴へ追加しない。

## 不明点
- `sessionlifecycle` を graph 全体のどの順序で接続するかは、この component 配下の実装だけでは不明です。
- `write_diary` tool 自体の本文生成ルールや保存形式の詳細は、この component 配下の実装だけでは確定できません。

## 参照元
- `internal/components/sessionlifecycle/sessionlifecycle.go`
- `internal/components/sessionlifecycle/sessionlifecycle_test.go`
- `internal/components/conversation/context_provider.go`
- `internal/components/conversation/rule_session_clear.go`
- `internal/components/conversation/rule_tool_response.go`
- `internal/types/types.go`
- `internal/types/event.go`
- `docs/components/conversation.md`
- `git show HEAD^:docs/9.タイマー・日記・自動リセット.md`
