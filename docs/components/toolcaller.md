# toolcaller component

`toolcaller` は `EventToolRequest` を受け取り、local tool handler を実行する component です。
tool 実行結果は downstream event では返さず、`conversationcommitter.ResultAPI.CommitToolResult` を呼んで会話コミッターへ戻します。

## 入力

- `EventToolRequest`
  - payload は `types.ToolRequest`
  - `Name` は tool 名
  - `Arguments` は JSON 引数
  - `GenerationID` は tool request が属する世代

## 出力

- tool 実行結果は `EventToolResponse` ではなく commit API で返す。
- `EventEmitterAware` な tool が発火する内部eventは従来通り downstream へ流せる。
- `whiteboard_update` のような UI 副作用eventは tool result とは別物として扱う。

## 現在の登録方針

このリファクタリング時点では、`cmd/smart-speaker/main.go` から `toolcaller.NewStage(nil, resultCommitter)` を呼び、登録済み tool は0件にしている。
既存 tool の移行は後続作業で行う。

## エラー処理

- 未登録 tool は `{"error":"unknown function: ..."}` を tool result として commit する。
- 引数 decode に失敗した場合は log を出し、空 map で実行する。
- handler error は error payload に正規化して commit する。
