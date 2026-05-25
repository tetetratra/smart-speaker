# toolcaller 概要理解ドキュメント

## 1. ビジネスコンテキスト

- **解決する課題**: LLM が生成した `tool` item を、会話生成パイプライン内で local tool 実行へ橋渡しする。
- **提供価値**: tool 実行結果を会話履歴へ戻し、必要に応じて LLM の次の推論へつなげる。UI 副作用を持つ tool は、tool result とは別に graph event として下流へ通知できる。
- **責務の境界**: `toolcaller` は tool の実行と tool result 保存要求 event の発行を担う。tool request の生成は `scheduler` / `router`、結果の履歴保存と LLM 再投入は `conversationcommitter` の責務。
- **通常起動での handler 登録**: `cmd/smart-speaker/main.go` は `buildToolRegistry` で local tool registry を構築し、`registry.Handlers()` を `toolcaller.NewStage(toolHandlers)` に渡す。これにより通常 pipeline でも登録済み local tool が実行対象になる。
- **根拠コード**: `internal/components/toolcaller/toolcaller.go`、`cmd/smart-speaker/main.go`、`internal/tools/registry/registry.go`、`internal/types/event.go`、`internal/types/conversation_record.go`。

## 2. 論理構造・機能俯瞰

**主要なモデル・コンポーネント**

- **`toolcaller.NewStage`**
  - `handlers map[string]tools.Handler` を受け取り、graph stage を作る。
  - `handlers == nil` の場合は空 map に正規化する。
  - stage の `Upstream` / `Downstream` はどちらも `graph.DefaultChannelBufferSize` の channel。

- **`toolCaller`**
  - `EventToolRequest` を購読し、payload が `types.ToolRequest` の場合だけ処理する。
  - `EventToolRequest` 以外の event は無視する。
  - tool 実行は `dispatchTool` で goroutine 化され、複数 request を並行実行できる。
  - stage close 時は context cancel 後、受信 goroutine と実行中 task の終了を待ってから `downstream` を close する。

- **`tools.Handler`**
  - `Name() string` と `Run(args map[string]any) (map[string]any, error)` を持つ local tool の実体。
  - `toolcaller` は `ToolRequest.Name` を key に `tools.Handler` を検索する。
  - handler が存在しない場合も失敗 event は出さず、`{"error":"unknown function: <name>"}` を tool result として commit する。

- **`tools.ContextAware`**
  - handler が実装している場合、`toolcaller.run` の開始時に `SetContext(ctx)` で stage context を渡す。
  - どの tool が実装しているかは、今回確認した範囲では不明。

- **`tools.EventEmitterAware`**
  - handler が実装している場合、`SetEventEmitter(s.emit)` で downstream event emitter を渡す。
  - 確認できる実装例は `internal/tools/functions/whiteboard/tool.go` の `set_whiteboard`。`Run` 内で `EventWhiteboardUpdate` を emit する。

- **`types.ToolRequest`**
  - `EventToolRequest` の payload。
  - フィールドは `ResponseID`、`ToolCallID`、`Name`、`Arguments`、`GenerationID`、`SequenceID`。
  - `scheduler` は `TimelineKindTool` から `ToolRequest` を作る。このとき `ToolCallID` と `SequenceID` には timeline の `SequenceID` が入る。

- **`types.ToolResultRecord`**
  - tool 実行結果を履歴へ保存するための payload。
  - `toolcaller` が設定するのは `ToolCallID`、`Name`、`Output`、`GenerationID`。

## 3. 主要なデータフロー

### シナリオ: LLM の tool item が local tool 実行結果として会話履歴へ戻るまで

1. LLM が tool timeline item を出力する: `llm` は `TimelineKindTool` の `types.TimelineItem` を graph に流す。
2. 世代 filter を通過する: `generationfilter` は `TimelineItem.GenerationID` が現在世代である場合だけ通す。
3. scheduler が実行要求へ変換する: `scheduler` は `TimelineKindTool` を `types.ToolRequest` に変換し、`EventScheduledItem` として出力する。
4. router が toolcaller へ振り分ける: `router` は `EventScheduledItem` の payload が `types.ToolRequest` の場合、同じ payload を `EventToolRequest` として出力する。
5. toolcaller が request を受け取る: `toolcaller` は `EventToolRequest` だけを処理し、payload が `types.ToolRequest` でなければ log 出力して無視する。
6. 引数を decode する: `ToolRequest.Arguments` が空でなければ JSON object として `map[string]any` へ unmarshal する。失敗した場合は log を出し、空 map で処理を続ける。
7. handler を実行する: `ToolRequest.Name` に対応する handler があれば `Run(args)` を呼ぶ。未登録の場合は unknown function error、handler error の場合は error 文字列を map に入れる。
8. 結果を JSON 化する: `map[string]any` を `json.Marshal` し、失敗した場合は `{"error":"result encoding failed"}` に置き換える。
9. tool result 保存要求を emit する: `types.ToolResultRecord` を作り、`types.ConversationCommitRequest{Role: RoleToolResult, ToolResult: &result}` を `EventConversationCommitRequest` として downstream へ送る。
10. graph が保存要求を転送する: 通常 pipeline では `toolcaller -> conversationcommitter` が `EventConversationCommitRequest` で接続される。
11. 会話履歴へ保存される: `conversationcommitter` は `conversationhistory.NewRecord` を経由して tool result を履歴化し、trim 後の text が空でなければ `EventLLMRequest` を `Role: "tool_result"` で発行する。

```mermaid
sequenceDiagram
    participant LLM as llm
    participant GF1 as generationfilter
    participant SCH as scheduler
    participant GF2 as generationfilter-scheduler
    participant R as router
    participant TC as toolcaller
    participant H as tools.Handler
    participant CC as conversationcommitter

    LLM->>GF1: EventTimelineItem(TimelineKindTool)
    GF1->>SCH: EventTimelineItem(TimelineKindTool)
    SCH->>GF2: EventScheduledItem(ToolRequest)
    GF2->>R: EventScheduledItem(ToolRequest)
    R->>TC: EventToolRequest(ToolRequest)
    TC->>H: Run(args)
    H-->>TC: map[string]any / error
    TC->>CC: EventConversationCommitRequest(RoleToolResult, ToolResult)
    CC-->>LLM: EventLLMRequest(RoleToolResult)
```

### シナリオ: EventEmitterAware tool が UI 副作用 event を出す場合

1. `toolcaller.run` は handler 初期化時、`tools.EventEmitterAware` を実装している tool に `s.emit` を渡す。
2. tool の `Run` 内で event が emit される。確認できる例では `set_whiteboard` が `EventWhiteboardUpdate` を出す。
3. `toolcaller.emit` は stage context がキャンセルされていなければ `toolcaller.Downstream` へ event を送る。
4. 通常 graph では `cmd/smart-speaker/main.go` により `toolcaller` から `wschat` へ `EventWhiteboardUpdate` だけが接続される。
5. `wschat` は `EventWhiteboardUpdate` を websocket message `{ "type": "whiteboard_update", "content": ... }` として接続 client に送る。
6. この UI 副作用 event は tool result commit とは別経路であり、`ToolResultRecord.Output` の代替ではない。

```mermaid
sequenceDiagram
    participant TC as toolcaller
    participant WB as set_whiteboard
    participant WS as wschat
    participant Client as WebSocket client

    TC->>WB: SetEventEmitter(s.emit)
    TC->>WB: Run(args)
    WB->>TC: emit(EventWhiteboardUpdate)
    TC->>WS: EventWhiteboardUpdate
    WS->>Client: whiteboard_update
```

## 4. 詳細設計

### クラス設計

- internal/
  - components/
    - toolcaller/
      - toolcaller.go: `EventToolRequest` を受けて local tool handler を実行し、結果を `EventConversationCommitRequest` として downstream へ流す stage。
        - `NewStage`: handler map から graph stage を生成する。nil handler map は空 map にする。
        - `run`: stage context を作り、`ContextAware` / `EventEmitterAware` handler へ依存を注入したうえで upstream を購読する。
        - `dispatchTool`: 1 request ごとに goroutine を起動し、tool 実行と result commit event の emit を行う。
        - `executeTool`: JSON 引数 decode、handler lookup、handler 実行、error 正規化、結果 JSON encode、`ToolResultRecord` 生成を行う。
        - `close`: context cancel、受信 goroutine 待機、upstream close を行う。
        - `emit`: `EventEmitterAware` tool から受けた event を context cancel を尊重して downstream へ流す。
      - toolcaller_test.go: 未登録 tool が空 output ではない `ToolResultRecord` を含む `EventConversationCommitRequest` として downstream に出ることを検証する。
  - types/
    - event.go: graph event と `EventToolRequest`、`ToolRequest` を定義する。
    - conversation_record.go: `ToolResultRecord` と `ConversationCommitRequest` を定義する。
  - components/
    - scheduler/
      - stage.go: `TimelineKindTool` を `ToolRequest` に変換し、`EventScheduledItem` として出す。
    - router/
      - stage.go: `EventScheduledItem` の payload が `ToolRequest` の場合、`EventToolRequest` として `toolcaller` へ渡す。
    - conversationcommitter/
      - committer.go: tool result を履歴保存し、空でない場合は `EventLLMRequest` を発行する。
    - wschat/
      - wschat.go: `EventWhiteboardUpdate` を websocket の `whiteboard_update` message に変換する。
  - tools/
    - interfaces.go: `Handler`、`ContextAware`、`EventEmitterAware`、`DefinitionProvider` を定義する。
    - registry/
      - registry.go: tool definitions と handler map を構築する。通常起動では `cmd/smart-speaker/main.go` の `buildToolRegistry` 経由で `llm` と `toolcaller` に接続される。
    - functions/
      - whiteboard/
        - tool.go: `EventEmitterAware` の確認済み実装。`set_whiteboard` 実行時に `EventWhiteboardUpdate` を emit する。
- cmd/
  - smart-speaker/
    - main.go: 通常 pipeline の stage 生成と graph 接続を定義する。`toolcaller.NewStage(toolHandlers)` に registry 由来の handler map を渡す。`router -> toolcaller` は `EventToolRequest`、`toolcaller -> conversationcommitter` は `EventConversationCommitRequest`、`toolcaller -> wschat` は `EventWhiteboardUpdate` で接続する。

### Event 設計

- **入力 event: `EventToolRequest`**
  - payload: `types.ToolRequest`
  - 生成元: `router`
  - `toolcaller` での扱い: payload 型が違う場合は log 出力して無視する。

- **副作用 event: `EventWhiteboardUpdate`**
  - payload: `types.WhiteboardUpdate`
  - 生成元: `EventEmitterAware` tool の一例である `set_whiteboard`
  - graph 接続: 通常 pipeline では `toolcaller -> wschat`
  - 注意: tool 実行結果を表す event ではなく、UI 更新用の別経路 event。

- **commit 用 event: `EventConversationCommitRequest`**
  - `toolcaller` の downstream から出る。
  - payload は `types.ConversationCommitRequest{Role: types.RoleToolResult, GenerationID: result.GenerationID, Source: result.Name, ToolResult: &result}`。
  - 通常 pipeline では graph が `conversationcommitter` へ転送する。

- **再推論 event: `EventLLMRequest`**
  - `toolcaller` は直接出さない。
  - `conversationcommitter` が tool result を履歴保存した後、trim 済み text が空でなければ発行する。

### Handler 登録設計

- `toolcaller.NewStage` の第 1 引数が実行可能な handler map である。
- map key は `ToolRequest.Name` と照合される。`registry.Handlers()` は `handler.Name()` を key にした map を返す。
- registry で handler が nil の entry は handler map に含まれない。
- `web_search` は `OPENAI_API_KEY` と `OPENAI_RESPONSES_MODEL` が registry config に渡される通常起動で登録される。LLM には local tool schema として提示され、handler map にも `web_search` key で入る。
- `web_search` handler は `query` 以外の引数を拒否し、OpenAI Responses API hosted `web_search` の回答本文を `{"result":"..."}` として commit 対象にする。citations や sources は tool result には含めない。
- 通常起動では `cmd/smart-speaker/main.go` の `buildToolRegistry` が registry を作り、`registry.Definitions()` を `llm.Config.ToolSchemas` に、`registry.Handlers()` を `toolcaller.NewStage` に渡す。
- SwitchBot tool は `SWITCHBOT_TOKEN` と `SWITCHBOT_SECRET` が揃う場合だけ登録される。scene 一覧取得に失敗した場合は `switchbot_execute_scene` のみ未登録になり、Hub 2 tool は token/secret が揃っていれば残る。

### エラー処理・副作用

- 引数 JSON decode error: log 出力し、空 map で handler 実行を継続する。
- unknown tool: `{"error":"unknown function: <name>"}` を `ToolResultRecord.Output` に入れる。
- handler error: `{"error":"<err.Error()>"}` を `ToolResultRecord.Output` に入れる。
- result JSON encode error: `{"error":"result encoding failed"}` を `ToolResultRecord.Output` に入れる。
- commit event emit: stage context が cancel 済みの場合は送信せずに終了する。
- `conversationcommitter` 側の副作用: tool result は会話履歴に保存され、空でなければ `EventLLMRequest` として LLM に戻る。

### テーブル設計

該当なし。確認した実装範囲では、`toolcaller` が直接参照・更新する DB テーブルはない。

### API設計

外部 HTTP API は該当なし。内部 API として追加の専用 API は使わず、graph の `EventConversationCommitRequest` を使う。

- `EventConversationCommitRequest`: tool 実行結果を会話履歴 commit request として `conversationcommitter` に戻す。
  - 入力: `types.ToolResultRecord{ToolCallID, Name, Output, GenerationID}`
  - payload: `types.ConversationCommitRequest{Role: types.RoleToolResult, GenerationID: result.GenerationID, Source: result.Name, ToolResult: &result}`
  - graph 接続: `toolcaller -> conversationcommitter`
