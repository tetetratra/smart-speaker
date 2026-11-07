# set_whiteboard（ホワイトボード追記）

## 概要

この文書は、ホワイトボード追記機能の現行実装を整理したものです。`set_whiteboard` は Realtime API の function calling で呼び出す通常の local tool で、入力・出力・`EventWhiteboardUpdate`・UI 反映を対象とします。

## 呼び出し方

- ツール定義は `internal/tools/functions/whiteboard/tool.go` の `Definition` で、`registry.DefinitionsForRealtime()` を通して `session.update` で Realtime API に渡します。
- Realtime API が `set_whiteboard` の function call を返すと、realtime が `EventToolRequest` として `toolcaller` に渡し、`toolcaller` が handler を実行します。他の local tool と同じ経路です。
- 応答方針（`tools.DefinitionWithResponsePolicy`）は、成功後も再び応答を生成する設定（`OnSuccess: true`）です。あわせて、表示完了そのものの報告や同じ内容の再更新はせず、未説明の取得結果だけを短く説明し、表示だけの依頼なら `wait_for_user` で待つよう追加指示します。

## できること

- アプリ画面のホワイトボード表示へ情報を追記します。
- 用途としては、予定、注意事項、要点など、口頭だけでは伝わりにくい情報を画面に残すことが想定されています。
- system prompt では、返答や感想ではなく、表示用の簡潔な内容だけを書くよう案内されています。
- 情報取得系 tool の結果を画面に残す場合は、口頭で説明するより先に `set_whiteboard` を呼ぶよう案内されています。

## 入力（handler）

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
- tool 定義の `x_tool_mode` は `write` です（現在の処理では参照されていません）。
- 成功・失敗のどちらでも、tool result は `toolcaller` から realtime に返り、会話履歴への記録と `function_call_output` による API への返却が行われます。画面反映は、これとは別の `EventWhiteboardUpdate` 経路で行われます。

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

1. Realtime API が `set_whiteboard` の function call を返し、realtime が `toolcaller` に渡します。
2. `toolcaller` が handler を実行します。
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

## 参照元

- [internal/components/realtime/events.go](/internal/components/realtime/events.go)
- [internal/components/toolcaller/toolcaller.go](/internal/components/toolcaller/toolcaller.go)
- [internal/tools/registry/registry.go](/internal/tools/registry/registry.go)
- [internal/tools/functions/whiteboard/tool.go](/internal/tools/functions/whiteboard/tool.go)
- [internal/components/wschat/wschat.go](/internal/components/wschat/wschat.go)
- [internal/types/event.go](/internal/types/event.go)
- [internal/types/types.go](/internal/types/types.go)
- [internal/app/system_prompt.go](/internal/app/system_prompt.go)
