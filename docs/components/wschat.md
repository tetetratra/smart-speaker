# wschat component

`wschat` component は `/ws/chat` を提供し、ブラウザとの JSON WebSocket 境界を担当します。

## 入力

- ブラウザからの `webrtc.*` message
  - `EventRTCSignal` に変換して downstream へ流す。
- graph からの UI 向け event
  - `EventRealtimeOutput`: user / assistant の表示message
  - `EventRTCSignal`: signaling 応答
  - `EventSpeechEnd`: 発話終了通知
  - `EventRTCVADStatus`: VAD 状態
  - `EventWhiteboardUpdate`: whiteboard 更新

## 表示しないもの

- `EventToolRequest` は通常の会話UIへ表示しない。
- tool result は通常の会話UIへ表示しない。
- STT の raw `EventHumanUtterance` は表示せず、発話バッファ後に commit された user message を表示する。
