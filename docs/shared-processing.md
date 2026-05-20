# 共通処理の再設計メモ

## 1. 目的
- `graph`、`event`、`stage` をまたぐ共通処理の責務を整理し、再設計時の分割方針を明確にする。
- 対象は「component 間の連携」「共通のデータ受け渡し」「責務境界」であり、個別機能の仕様は扱わない。
- 不明な点は推測せず `不明` と記載する。

参照:
- `cmd/smart-speaker/main.go`
- `internal/graph/graph.go`
- `internal/graph/stage.go`
- `internal/graph/eventlog.go`
- `internal/types/event.go`
- `internal/types/types.go`
- `git show HEAD^:docs/1.全体アーキテクチャとイベントグラフ.md`
- `git show HEAD^:docs/6.ツール実行基盤.md`

## 2. 現在の共通処理の見取り図

```mermaid
flowchart LR
	main["main.go\nwireGraph"] --> graph["graph.Graph\n接続定義と転送"]
	graph --> stage["graph.Stage\n実行単位"]
	stage --> event["types.Event\n共通搬送体"]
	event --> payload["Payload\n各種ドメイン型"]
	payload --> components["conversation / responsesapi / rtc /\nwschat / toolcaller / tts"]
```

現在の共通処理は、主に以下の 4 層で成立している。

- `main.go`
  - Stage の生成、名前付け、依存注入、接続順の定義を担当する。
- `graph.Graph`
  - Node/Edge の保持と、`Downstream -> Upstream` のイベント転送を担当する。
- `graph.Stage`
  - 各 component を graph に載せるための最小単位を提供する。
- `types.Event` とその payload 型
  - component 間で共有されるデータ受け渡し契約を表す。

この構造により、各 component は他 component の実装詳細を直接知らず、`EventKind` と payload 型を介して連携している。

## 3. 現在の責務分割

### 3-1. `graph.Graph` の責務
- `AddNode` で Stage を登録する。
- `Connect` で接続だけを定義する。
- `Run` で各 Stage を起動し、各 edge をもとにイベントを downstream へ複製転送する。
- forward log を出すが、イベント内容の解釈は限定的である。

言い換えると、`Graph` は「ルーティング層」であり、業務判断や状態管理を持たない。

参照:
- `internal/graph/graph.go`
- `internal/graph/eventlog.go`

### 3-2. `graph.Stage` の責務
- `Upstream` と `Downstream` のチャネルを持つ。
- `Run(context.Context)` で component の実行本体を持つ。
- `Close()` で終了処理を 1 回だけ実行する。

`Stage` 自体は薄く、component のライフサイクルを graph に載せるためのアダプタとして使われている。

参照:
- `internal/graph/stage.go`

### 3-3. `types.Event` の責務
- `Kind` でイベント種別を識別する。
- `Payload` に具体データを積む。
- payload の実体は `OutputLine`、`ToolRequest`、`ResponsesRequest` などのドメイン型で表現される。

`Event` は統一搬送体だが、payload は `any` であるため、受信側で型アサーションが必要になる。

参照:
- `internal/types/event.go`
- `internal/types/types.go`

### 3-4. component の責務
- `main.go` の `wireGraph` 上、component 同士は多対多に接続されている。
- たとえば `conversation` は `responsesapi`、`tts`、`wschat`、`rtc` と双方向に接続される。
- `toolcaller` は `responsesapi`、`conversation`、`wschat` へ結果を渡す。

したがって現在の構造では、`graph` 自体は薄い一方、どの component がどのイベントを読むかという知識が `wireGraph` と各 component の両方に分散している。

参照:
- `cmd/smart-speaker/main.go`

## 4. 共通データ受け渡しの現状

### 4-1. 良い点
- 全 component が同一の `types.Event` を使うため、接続方法が統一されている。
- `ResponsesRequest`、`ToolRequest`、`RTCSignal` など、代表的な連携データは型として明示されている。
- `eventlog` により、主要イベントの転送ログを stage 名付きで追跡できる。

### 4-2. 制約
- `Payload any` のため、型安全性は compile time では担保されない。
- `EventKind` と payload 型の対応表がコード上の 1 箇所に閉じていない。
- 1 つのイベントが複数 component に broadcast されるため、「誰が購読者か」が `wireGraph` を読まないと分かりにくい。
- event forwarding は全接続に対して等しく行われ、購読条件や優先度の概念はない。

### 4-3. 現行で共通化されているデータ
- 制御イベント
  - `EventTTSCancel`
- 会話入力イベント
  - `EventHumanUtterance`
  - `EventSpeechEnd`
- 応答処理イベント
  - `EventResponsesRequest`
  - `EventResponsesResponse`
  - `EventResponsesStreamChunk`
- ツール連携イベント
  - `EventToolRequest`
  - `EventToolResponse`
- UI/音声向けイベント
  - `EventRealtimeOutput`
  - `EventRealtimeAudio`
  - `EventRTCSignal`
  - `EventWhiteboardUpdate`
- 監督用イベント
  - `EventRTCVADStatus`

参照:
- `internal/types/event.go`
- `internal/types/types.go`

## 5. 再設計で分けて考えるべき責務

### 5-1. 配線責務
対象:
- Stage の生成順
- 依存注入
- component 間接続

現状:
- `main.go` に集中している。

再設計方針:
- 配線責務は引き続き 1 箇所に集約した方がよい。
- ただし「接続定義」と「component 構築」は分離した方が追いやすい。
- 具体的な分離先は現状コードからは不明。

参照:
- `cmd/smart-speaker/main.go`

### 5-2. 搬送責務
対象:
- event の複製転送
- stage 実行・停止
- ログ整形

現状:
- `graph.Graph` と `graph.Stage` が担当している。

再設計方針:
- この層はドメイン知識を持たないまま維持するのが自然。
- 共通処理として持つなら、「運ぶだけ」「止めるだけ」に寄せる。
- event の詳細表示は補助機能であり、ルーティング本体と分離可能。

参照:
- `internal/graph/graph.go`
- `internal/graph/eventlog.go`
- `internal/graph/stage.go`

### 5-3. データ契約責務
対象:
- `EventKind`
- payload 型
- kind と payload の組み合わせ規約

現状:
- `types` に型はあるが、組み合わせ規約は慣習に近い。

再設計方針:
- 共通処理として最も整理効果が高いのはこの層である。
- 少なくとも「どの kind にどの payload 型を積むか」を一覧化できる形に寄せた方がよい。
- `Payload any` を維持するか、型付き envelope に寄せるかは設計判断が必要。どちらを採るべきかはこの参照範囲だけでは断定できない。

参照:
- `internal/types/event.go`
- `internal/types/types.go`

### 5-4. component 内責務
対象:
- 会話制御
- ツール実行
- UI 境界
- RTC 境界

現状:
- 各 component 内に閉じている。

再設計方針:
- 共通処理側に寄せすぎない方がよい。
- `graph`/`event`/`stage` は component 間の契約に留め、状態遷移や業務判断は各 component に残すべきである。
- これは旧 `docs/1.全体アーキテクチャとイベントグラフ.md` の「graph は薄く保つ」という方針とも整合する。

## 6. 推奨する境界の置き方

### 6-1. `graph`
- 持つべき責務
  - Stage の起動
  - Event の転送
  - 終了処理
- 持たせない方がよい責務
  - 特定イベントの意味解釈
  - component ごとの条件分岐
  - 業務ルール

### 6-2. `event`
- 持つべき責務
  - component 間連携に必要な最小データ契約
  - イベント種別の識別
- 持たせない方がよい責務
  - component 内部専用の細かい状態
  - 一時的な実装都合だけの値

### 6-3. `stage`
- 持つべき責務
  - 実行単位の共通インターフェース
  - 入出力チャネル
  - close 管理
- 持たせない方がよい責務
  - 共有状態の保有
  - ルーティングポリシー

### 6-4. component
- 持つべき責務
  - 状態管理
  - イベント解釈
  - 外部 I/O
  - 副作用実行

## 7. 再設計時の論点

### 論点 1: 配線の見通し
- 現在は `wireGraph` を読むと全体接続は追える。
- 一方で接続数が多く、双方向接続も多いため、責務境界の把握は容易ではない。
- 再設計では「どのイベントを誰が publish / subscribe するか」を接続図または一覧で補助したい。

### 論点 2: 型安全性
- `Payload any` は柔軟だが、受信側の暗黙知に依存する。
- 再設計では、少なくとも event 一覧と payload 一覧の対応表をドキュメントまたはコードで持つべきである。

### 論点 3: broadcast 前提
- 現在の `Graph.Run` は downstream 全件へ同一イベントを渡す。
- selective delivery や topic 分離が必要かは、この参照範囲だけでは不明。

### 論点 4: ログと本体責務の分離
- `eventlog` は実運用上有用だが、共通処理の本体ではない。
- 再設計時は、ルーティングと可観測性を分けて扱える構造の方が保守しやすい。

## 8. 不明点
- 各 component が実際にどの `EventKind` を購読し、どれを無視するかの完全一覧。
- `DefaultChannelBufferSize` がどこで使われているか、および共通バッファ戦略。
- 将来的に `graph` を topic ベースや request-response ベースへ拡張する意図があるか。
- `eventlog` の拡張ポイントを今後 public に保ちたいかどうか。

## 9. このメモの結論
- 現行の共通処理は、`main.go` の配線、`graph` の転送、`stage` の実行単位、`types.Event` の搬送契約に分かれている。
- 再設計で中心的に見直すべきなのは、`graph` の複雑化ではなく、`event` の契約整理と配線の見通しである。
- component の業務ロジックまで共通処理へ吸い上げるのではなく、共通処理は薄い基盤として維持する方針が妥当である。
