# timer（時間指定 action）

## 概要

`timer` は、未来の時刻に扱う自然言語 action をプロセス内メモリへ登録する
local tool です。登録済み timer の取消は `cancel_timer` tool で行います。
「20分後に起こして」「21時になったらエアコンをoffにして」
のような依頼を、比較可能な絶対時刻と自然言語 action の組として保持します。

保存済みの任意 tool call を直接実行するものではありません。期限到達時には、
保存した action を system commit として会話履歴へ渡し、LLM が通常の timeline
生成で speech や既存 tool call を選びます。

## 入力

- 登録 tool 名: `timer`
- 取消 tool 名: `cancel_timer`
- mode: `write`
- `timer` の必須引数: `operation`
- `operation=create` の必須引数: `at`, `action`
- `cancel_timer` の必須引数: `id`

`at` は RFC3339 の絶対時刻です。相対指定の解釈は、LLM が system prompt の現在日時
をもとに行います。

`action` には、期限到達時点で実行する内容だけを保存します。「10分後にエアコンをつけて」
のような依頼では、`10分後に` を `at` の解釈にだけ使い、`action` は
`エアコンをつける` として登録します。

登録例:

```json
{
  "operation": "create",
  "at": "2026-06-03T21:00:00+09:00",
  "action": "エアコンをoffにする"
}
```

取消例:

```json
{
  "id": "timer-id"
}
```

## 保存するデータ

`internal/states/timer.Store` は、次の値をメモリ上で保持します。

| フィールド | 内容 |
|---|---|
| `id` | タイマー ID |
| `at` | 期限到達時刻 |
| `action` | 期限到達時に AI へ渡す自然言語 action |
| `created_at` | 登録時刻 |

永続化は行いません。プロセス再起動時に登録済み timer は破棄されます。

## 期限到達時の処理

`timer` tool は `ContextAware` / `EventEmitterAware` として動作し、toolcaller から
context と event emitter が注入されると軽量な ticker で期限到達を監視します。

期限に到達した timer は Store から取り出され、次のような system commit として
`EventConversationCommitRequest` に流されます。

```text
タイマーの期限に到達しました。at=2026-06-03T21:00:00+09:00 action=エアコンをoffにする
```

`conversationcommitter` は system commit を履歴へ保存し、`EventLLMRequest` を発行
します。これにより、期限到達後の発話や家電操作は LLM の通常応答として処理されます。

## AI コンテキスト

LLM stage は request ごとに未到達 timer の snapshot を system prompt へ追記します。
この snapshot は現在状態を表すため、会話履歴へ timer 一覧を積み続ける必要はありません。

prompt へ追記される形式:

```text
現在の未到達タイマー一覧:
- id=timer-id at=2026-06-03T21:00:00+09:00 action=エアコンをoffにする
```

未登録の場合は `- なし` を渡します。

## 管理画面との同期

timer の登録、取消、期限到達時には `EventTimerState` が発行されます。通常 pipeline では
`toolcaller -> wschat` がこの event を受け取り、WebSocket message
`timer.state` として接続中クライアントへ配信します。

```json
{
  "type": "timer.state",
  "timers": [
    {
      "id": "timer-id",
      "at": "2026-06-03T21:00:00+09:00",
      "action": "エアコンをoffにする",
      "created_at": "2026-06-03T10:00:00Z"
    }
  ]
}
```

管理画面では未到達 timer の件数、期限、action、id を表示します。取消操作 UI は持たず、
自然発話から `cancel_timer` を呼ぶ想定です。

## 制約

- timer はプロセス内メモリにのみ保存されます。
- 再起動後の復元、リトライ、失敗状態の永続管理は行いません。
- 登録済みの tool call を直接実行しません。
- `action` は自然言語のまま保存されるため、期限到達後の実際の応答・操作は LLM の解釈に委ねられます。

## 参照元

- [internal/states/timer/store.go](/internal/states/timer/store.go)
- [internal/tools/functions/timer/tool.go](/internal/tools/functions/timer/tool.go)
- [internal/components/llm/stage.go](/internal/components/llm/stage.go)
- [internal/components/llm/responses_client.go](/internal/components/llm/responses_client.go)
- [internal/components/conversationcommitter/committer.go](/internal/components/conversationcommitter/committer.go)
- [internal/components/wschat/wschat.go](/internal/components/wschat/wschat.go)
- [web/src/main.tsx](/web/src/main.tsx)
