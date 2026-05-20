# wschat component

## 概要
`wschat` component は、ブラウザとの JSON WebSocket 境界を担当する component です。`/ws/chat` を提供し、ブラウザから受けた WebRTC signaling message を internal event に変換し、逆に internal event を UI 向け JSON message として配信します。

## 責務
- `/ws/chat` endpoint を提供する。
- WebSocket 接続ごとに `ClientID` を払い出して接続を管理する。
- ブラウザからの `webrtc.*` message を `EventRTCSignal` に変換する。
- graph から受けた会話・tool・speech・VAD・whiteboard・WebRTC signaling event を UI 向け JSON として送信する。
- signaling は対象 `ClientID` がある接続にだけ返し、それ以外の UI 向け event は全接続へ broadcast する。

## 担当範囲
- ブラウザとの WebSocket 境界
- メッセージ変換
- 接続管理
- signaling 中継
- UI への配信

## 担当しないこと
- WebRTC `PeerConnection` 自体の生成と音声トラック処理
- assistant 応答生成
- tool 実行
- UI 描画
- 再接続方針やブラウザ側 state 管理

## 入力

### ブラウザから受ける WebSocket message
| `type` | 主な項目 | 内部変換 |
| --- | --- | --- |
| `webrtc.offer` | `sdp` | `types.Event{Kind: EventRTCSignal, Payload: types.RTCSignal{Type, SDP, ClientID}}` |
| `webrtc.answer` | `sdp` | `types.Event{Kind: EventRTCSignal, Payload: types.RTCSignal{Type, SDP, ClientID}}` |
| `webrtc.ice` | `candidate` | `types.Event{Kind: EventRTCSignal, Payload: types.RTCSignal{Type, Candidate, ClientID}}` |

### graph から受ける event
| EventKind | payload | UI 向け変換 |
| --- | --- | --- |
| `EventRealtimeOutput` | `types.OutputLine` | `type: "message"` |
| `EventHumanUtterance` | `types.OutputLine` | `type: "message"` |
| `EventToolRequest` | `types.ToolRequest` | `type: "function_call"` |
| `EventToolResponse` | `types.ToolResponse` | `type: "function_result"` |
| `EventRTCSignal` | `types.RTCSignal` | `type: sig.Type` |
| `EventSpeechEnd` | `types.SpeechEvent` | `type: "speech_end"` |
| `EventRTCVADStatus` | `types.RTCVADStatus` | `type: "rtc_vad_status"` |
| `EventWhiteboardUpdate` | `types.WhiteboardUpdate` | `type: "whiteboard_update"` |

## 出力

### graph へ出す event
| EventKind | payload | 出力条件 |
| --- | --- | --- |
| `EventRTCSignal` | `types.RTCSignal` | 受信 message の `type` が `webrtc.` で始まるとき。 |

### ブラウザへ返す WebSocket message
| `type` | 主な項目 | 送信先 |
| --- | --- | --- |
| `message` | `role`, `text`, `response_id`, `final`, `source` | 全接続 broadcast |
| `function_call` | `tool_call_id`, `name`, `arguments`, `response_id` | 全接続 broadcast |
| `function_result` | `tool_call_id`, `name`, `output` | 全接続 broadcast |
| `speech_end` | `source`, `captured_at` | 全接続 broadcast |
| `rtc_vad_status` | `input_level`, `threshold`, `captured_at` | 全接続 broadcast |
| `whiteboard_update` | `content` | 全接続 broadcast |
| `webrtc.answer` | `sdp` | `RTCSignal.ClientID` で特定された接続だけに送信 |
| `webrtc.ice` | `candidate` | `RTCSignal.ClientID` で特定された接続だけに送信 |

## 接続管理
- `connHolder` が `map[string]*websocket.Conn` を保持し、`sync.RWMutex` で保護する。
- 新規接続時に `atomic.AddUint64` で連番を進め、`ws-1` のような `ClientID` を払い出す。
- 接続は `holder.add` で登録し、handler 終了時に `holder.remove` で close と削除を行う。
- stage close 時は `holder.clearAll` が全接続を `StatusNormalClosure` で閉じる。

## 主要フロー

### シナリオ: ブラウザの signaling が rtc へ渡るまで
1. ブラウザが `/ws/chat` へ `webrtc.offer` または `webrtc.ice` を送る。
2. `wschat` が `type` の接頭辞 `webrtc.` を検出する。
3. 現在の接続に対応する `ClientID` を付けた `types.RTCSignal` を組み立てる。
4. `EventRTCSignal` として downstream へ出す。

### シナリオ: graph の event が UI に表示されるまで
1. `wschat` が upstream から event を受ける。
2. event kind ごとに UI 向け JSON message を組み立てる。
3. `json.Marshal` で text frame に変換する。
4. signaling で `ClientID` がある場合はその接続だけに送る。
5. それ以外は `connHolder.snapshot()` の全接続へ送る。

## メッセージ変換ルール
- `EventRealtimeOutput` は `message` に変換され、`response_id` と `final` を含む。
- `EventHumanUtterance` は `message` に変換されるが、`response_id` と `final` は含まない。
- `EventToolRequest.Arguments` は `json.RawMessage` のまま `function_call.arguments` に入る。
- `EventToolResponse.Output` は `json.RawMessage` のまま `function_result.output` に入る。
- `EventSpeechEnd` の `captured_at` は `RFC3339Nano` 文字列に変換される。
- `EventRTCSignal` は `sig.Type` をそのまま UI message の `type` に使う。

## 実装上の注意
- WebSocket accept では `websocket.AcceptOptions{InsecureSkipVerify: true}` を使っている。
- `handleEvent` は payload 型が期待と一致しない event を黙って破棄する。
- `writeMessage` の `conn.Write` エラーは呼び出し側へ返さず無視する。
- broadcast 時は `connHolder.snapshot()` で接続一覧をコピーしてから送る。
- `EventRTCSignal` でも `ClientID` が空なら broadcast になる。

## 境界上の前提
- ブラウザから送る signaling の payload 形式は `sdp` または `candidate` を含む JSON である。
- `wschat` 自体は message schema のバージョニングや後方互換管理を持たない。

## 不明点
- `webrtc.answer` をブラウザから server へ送る経路が現行通常フローで必要かどうかは、`wschat` 単体の実装と指定された旧 docs だけでは不明です。
- 複数ブラウザ接続が同時に存在するとき、通常 message を全接続 broadcast することが最終仕様として意図されたものかどうかは不明です。

## 参照元
- `internal/components/wschat/wschat.go`
- `internal/types/event.go`
- `internal/types/types.go`
- `git show HEAD^:docs/1.全体アーキテクチャとイベントグラフ.md`
- `git show HEAD^:docs/4.Webフロントと接続ライフサイクル.md`
