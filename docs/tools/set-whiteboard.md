# set_whiteboard tool

## 概要
この文書は、`set_whiteboard` tool の現行実装ベースの挙動を、再設計の前提として整理したものです。対象範囲は tool の入力、出力、`EventWhiteboardUpdate`、UI 反映経路に限定します。

## できること
- アプリ画面のホワイトボード表示へ情報を追記します。
- 用途としては、予定、注意事項、要点など、口頭だけでは伝わりにくい情報を画面に残すことが想定されています。
- tool description では、返答や感想ではなく、表示用の簡潔な内容だけを書くよう案内されています。

## 入力
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
1. LLM が `set_whiteboard` を `content` 付きで呼び出します。
2. `internal/tools/functions/whiteboard/tool.go` の `Run` が入力を正規化します。
3. `Run` が `EventWhiteboardUpdate` を emit します。
4. `internal/components/wschat/wschat.go` の `handleEvent` が `EventWhiteboardUpdate` を受け取ります。
5. `wschat` は payload を `whiteboard_update` という WebSocket メッセージに変換します。
6. 変換後のメッセージは、特定クライアント指定なしで接続中クライアント全体へ送信されます。
7. フロントエンドは空でない `content` をホワイトボードの新しい entry として末尾へ追加します。
8. 通常画面では entry 間に罫線を表示し、追記時にホワイトボードのスクロール位置を末尾へ移動します。

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

## 不明
- 再設計後に `content` 以外の構造化フィールドを持たせる方針があるかは、この参照範囲からは不明です。

## 参照元
- [internal/tools/functions/whiteboard/tool.go](/internal/tools/functions/whiteboard/tool.go)
- [internal/components/wschat/wschat.go](/internal/components/wschat/wschat.go)
- [internal/types/event.go](/internal/types/event.go)
- [internal/types/types.go](/internal/types/types.go)
- 旧 docs: `git show HEAD^:docs/8.生活操作ツール群.md`
- 旧 docs: `git show HEAD^:docs/6.ツール実行基盤.md`
