# set_whiteboard（ホワイトボード追記）

## 概要

この文書は、ホワイトボード追記機能の現行実装を整理したものです。LLM 向けの出力契約（root 追加フィールド）と、内部で利用する `set_whiteboard` tool の挙動（入力・出力・`EventWhiteboardUpdate`・UI 反映）を対象とします。

## LLM 出力契約

ホワイトボード追記は、JSON timeline の **root 任意フィールド** `set_whiteboard` で行います。`items` 配列内の tool としては出しません。

```json
{
  "set_whiteboard": { "content": "表示用テキスト" },
  "items": [
    { "type": "speech", "text": "..." }
  ]
}
```

| 観点 | 内容 |
|------|------|
| フィールド位置 | root object（`items` と同階層） |
| 必須 | いいえ（省略可） |
| 形状 | `{ "content": "..." }`（非空文字列。前後空白は trim 後に検証） |
| `items` 内 tool | **禁止**（パーサがエラーにする） |
| Structured Outputs schema | root に `set_whiteboard` プロパティを定義。`items` の tool `anyOf` には含めない |

### パース時の統合

`internal/components/llm/contract.go` の `parseTimelineJSON` は、`set_whiteboard` フィールドがある場合、検証後に `items` の **先頭** へ `TimelineKindTool`（`tool_name=set_whiteboard`）を挿入し、全 item の `SequenceID` を `1` から振り直します。

- 下流（scheduler / toolcaller / whiteboard handler）は、従来どおり tool として処理します。
- #158 対応により、先頭 tool は他 item より先に実行され、ホワイトボード反映を早めます。
- LLM 向け tool 定義は `registry.DefinitionsForLLM()` で `set_whiteboard` を除外します。handler は `registry.Handlers()` に残します。

## できること

- アプリ画面のホワイトボード表示へ情報を追記します。
- 用途としては、予定、注意事項、要点など、口頭だけでは伝わりにくい情報を画面に残すことが想定されています。
- system prompt では、返答や感想ではなく、表示用の簡潔な内容だけを書くよう案内されています。
- GET 系 tool の結果を画面に残す場合は、root の `set_whiteboard` に書き、続けて `items` に speech や他 tool を並べる運用を推奨しています。

## 内部 tool の入力（handler）

パース後、内部では次の tool として実行されます。

- tool 名: `set_whiteboard`
- 必須引数: `content`
- 型: `string`
- `parameters.required` には `content` のみが含まれます。
- `additionalProperties` は `false` です。

```json
{
  "type": "object",
  "properties": {
    "content": {
      "type": "string",
      "description": "ホワイトボードに追記する文章。7行程度を目安にし、URLやリンク付きテキストは含めないでください。"
    }
  },
  "required": ["content"],
  "additionalProperties": false
}
```

### 入力の正規化

- `args["content"]` が `string` のときだけ値として扱います。
- 前後空白は `strings.TrimSpace` で除去されます。
- 文字列中の `\\n` は実際の改行文字 `\n` に変換されます。
- 改行変換後に再度 `TrimSpace` が適用されます。
- 正規化後の `content` が空文字なら `content is required` エラーを返します。

## 出力

成功時の handler 返り値は次の形式です。

```json
{
  "content": "表示内容",
  "updated": true
}
```

- `content` には、正規化後の文字列がそのまま入ります。
- `updated` は常に `true` です。
- tool 定義の `x_tool_mode` は `write` です。
- 成功時の tool result は `toolcaller` が会話履歴へ commit せず、LLM への再投入も行いません。画面反映は `EventWhiteboardUpdate` 経路で行われます。
- エラー時は従来どおり tool result が LLM へ返ります。

## EventWhiteboardUpdate

- `set_whiteboard` は、返り値とは別に内部 event を発火します。
- event kind は `EventWhiteboardUpdate` です。
- payload 型は `types.WhiteboardUpdate` です。

```go
types.Event{
	Kind: types.EventWhiteboardUpdate,
	Payload: types.WhiteboardUpdate{
		Content: content,
	},
}
```

- `WhiteboardUpdate` が持つフィールドは、参照範囲では `Content string` のみです。
- event emitter が未注入のときは event は発火されませんが、tool の返り値自体は成功として返ります。

## UI 反映経路

1. LLM が root に `set_whiteboard` を `content` 付きで出力します（またはパース後の内部表現として先頭 tool item になる）。
2. `internal/components/llm/contract.go` が必要に応じて先頭へ tool item を挿入します。
3. `internal/tools/functions/whiteboard/tool.go` の `Run` が入力を正規化します。
4. `Run` が `EventWhiteboardUpdate` を emit します。
5. `internal/components/wschat/wschat.go` の `handleEvent` が `EventWhiteboardUpdate` を受け取ります。
6. `wschat` は payload を `whiteboard_update` という WebSocket メッセージに変換します。
7. 変換後のメッセージは、特定クライアント指定なしで接続中クライアント全体へ送信されます。
8. フロントエンドは空でない `content` をホワイトボードの新しい entry として末尾へ追加します。
9. 通常画面では entry 間に罫線を表示し、追記時にホワイトボードのスクロール位置を末尾へ移動します。

WebSocket で送られるメッセージ形式:

```json
{
  "type": "whiteboard_update",
  "content": "表示内容"
}
```

## 制約

- tool description では 7 行程度を目安にするよう案内されていますが、コード上で行数制限はしていません。
- tool description では URL やリンク付きテキストを含めないよう案内されていますが、コード上で検証はしていません。
- WebSocket payload は差分ではなく、1回の追記内容だけを表します。
- `items` 内の `set_whiteboard` tool は後方互換なく拒否します。

## 参照元

- [internal/components/llm/contract.go](/internal/components/llm/contract.go)
- [internal/components/llm/schema.go](/internal/components/llm/schema.go)
- [internal/tools/registry/registry.go](/internal/tools/registry/registry.go)
- [internal/tools/functions/whiteboard/tool.go](/internal/tools/functions/whiteboard/tool.go)
- [internal/components/wschat/wschat.go](/internal/components/wschat/wschat.go)
- [internal/types/event.go](/internal/types/event.go)
- [internal/types/types.go](/internal/types/types.go)
- [system_prompt.txt](/system_prompt.txt)
