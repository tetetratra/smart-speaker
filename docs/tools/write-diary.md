# write_diary tool

## 概要
`write_diary` は、直近の会話を要約した本文を日記として永続化する function tool です。通常会話の途中でユーザーへ何かを返すための tool ではなく、主に会話終了時の記録を次回会話へ引き継ぐために使われます。

## 役割
- 入力された `content` を日記本文として `diary store` に追記する。
- 追記時刻を `time.Now()` で決め、保存先 path と timestamp を返す。
- 次回会話で参照される長期記憶の更新点になる。

### 現在の入力
`Definition()` で公開されている引数は 1 つです。

| 引数 | 型 | 必須 | 現在の扱い |
| --- | --- | --- | --- |
| `content` | `string` | 必須 | 空白のみは不可です。`\\n` は実改行に戻してから保存されます。 |

### 現在の出力
成功時の返り値は次の `map[string]any` です。

| キー | 型 | 内容 |
| --- | --- | --- |
| `path` | `string` | 実際に追記した日記ファイルの path |
| `timestamp` | `string` | 追記時刻。`time.RFC3339` 形式 |

## 通常会話から除外される理由
`write_diary` は registry には登録されていますが、通常会話用の `responsesapi.NewStage` には `toolRegistry.DefinitionsExcluding("write_diary")` で渡されます。そのため、通常の assistant 応答生成では LLM に公開されません。

除外されている理由として、コード上で確認できる事実は次のとおりです。

- `sessionlifecycle` が idle timeout 時にだけ `write_diary` を強制選択する設計になっている。
- `conversation` は `write_diary` の `ToolResponse` を会話履歴へ追加しない。
- `context_provider` は保存済み diary 全文を次回会話の system message として注入する。

つまり現在の `write_diary` は、その場の会話を進める tool ではなく、会話外の永続記憶を更新する専用 tool として分離されています。

## sessionlifecycle との関係
`write_diary` を実際に使う起点は `sessionlifecycle` です。`sessionlifecycle` は会話の最新 snapshot と最終活動時刻を保持し、一定時間無操作になったときに `write_diary` 専用の `EventResponsesRequest` を発行します。

### 発火条件
- 最新 snapshot が空でない。
- `WriteDiaryTools` に `write_diary` の定義が入っている。
- 最終活動時刻が設定されている。
- すでに diary 実行中ではない。
- idle threshold 経過済みである。

### リクエスト内容
- `Role`: `system`
- `Text`: 会話を日記にまとめる指示文
- `Messages`: idle 時点の会話 snapshot の clone
- `SystemPrompt`: 空文字
- `ToolChoice`: `write_diary` を強制指定
- `Tools`: `write_diary` の定義のみ

### 完了後の扱い
- `sessionlifecycle` は `EventToolResponse` の `Name` が `write_diary` なら完了として扱う。
- diary 実行中フラグが立っている場合だけ `EventSessionClear` を emit する。
- diary 実行中に新しい会話 activity が来た場合、古い `write_diary` 完了通知は無視される。

## diary store との関係
`write_diary` 自身はファイル操作を直接持たず、`DiaryAppender` interface 経由で `diary store` に書き込みます。標準実装は `internal/diary/store.go` の `Store` です。

### store 側の責務
- 保存先 path の決定
  - 既定値は `data/diary.md`
- diary ファイルの初回作成
- 旧 `tmp/diary/*.md` からの初回移行
- 既存 diary 末尾への追記
- `Content()` による diary 全文の読み出し

### 追記フォーマット
- 見出しは `# YYYY-MM-DD HH:MM`
- 本文は `content` の先頭改行を除去し、末尾改行を整えて保存する
- 既存内容がある場合は空行 2 つ区切りで追記する

## 次回会話での使われ方
保存された diary は `conversation` の `contextProvider` から読まれます。`Content()` の結果が空でなければ、`以下は過去の会話をまとめた日記です。参考として扱ってください。` という prefix 付きの system message として会話先頭へ挿入されます。

この経路により、`write_diary` の出力は当回の会話履歴には混ぜず、次回会話の参照コンテキストとしてだけ再利用されます。

## エラーと制約
- `content` が空白のみの場合は `content is required` を返す。
- store への追記に失敗した場合は `failed to write diary: ...` を返す。
- tool definition の description には「指示がない限り勝手に呼び出さないでください」とあるが、通常会話からはそもそも除外されている。
- 日記本文の内容品質は tool 自身では保証せず、要約文の生成は `sessionlifecycle` から送られた request を処理する LLM 側に委ねられる。

## 不明
- 再設計後も `data/diary.md` への単一ファイル追記を維持するかは不明です。
- diary を会話ごとに分割保存するか、検索可能な構造化 store に変更するかは不明です。
- `write_diary` を manual tool としてユーザー操作で呼べるようにするかは不明です。
- diary の保持期間や削除ポリシーは、この参照範囲からは不明です。

## 参照元
- [internal/tools/functions/diary/tool.go](/Users/kondo.daichi/p/smart-speaker/internal/tools/functions/diary/tool.go)
- [internal/tools/functions/diary/tool_test.go](/Users/kondo.daichi/p/smart-speaker/internal/tools/functions/diary/tool_test.go)
- [internal/diary/store.go](/Users/kondo.daichi/p/smart-speaker/internal/diary/store.go)
- [internal/components/sessionlifecycle/sessionlifecycle.go](/Users/kondo.daichi/p/smart-speaker/internal/components/sessionlifecycle/sessionlifecycle.go)
- [internal/components/sessionlifecycle/sessionlifecycle_test.go](/Users/kondo.daichi/p/smart-speaker/internal/components/sessionlifecycle/sessionlifecycle_test.go)
- [internal/components/conversation/context_provider.go](/Users/kondo.daichi/p/smart-speaker/internal/components/conversation/context_provider.go)
- [internal/components/conversation/conversation.go](/Users/kondo.daichi/p/smart-speaker/internal/components/conversation/conversation.go)
- [internal/components/conversation/rule_tool_response.go](/Users/kondo.daichi/p/smart-speaker/internal/components/conversation/rule_tool_response.go)
- [cmd/smart-speaker/main.go](/Users/kondo.daichi/p/smart-speaker/cmd/smart-speaker/main.go)
- `git show HEAD^:docs/9.タイマー・日記・自動リセット.md`
