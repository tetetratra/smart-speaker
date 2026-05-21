# wschat component

## 概要
`wschat` はブラウザとの JSON WebSocket 境界です。`/ws/chat` を提供し、WebRTC signaling を internal event に変換し、graph から受けた UI 向け event を JSON message として配信します。

## graph から受ける event
| EventKind | payload | UI 向け変換 |
| --- | --- | --- |
| `EventRealtimeOutput` | `types.OutputLine` | `type: "message"` |
| `EventHumanUtterance` | `types.OutputLine` | `type: "message"` |
| `EventRTCSignal` | `types.RTCSignal` | `type: sig.Type` |
| `EventSpeechEnd` | `types.SpeechEvent` | `type: "speech_end"` |
| `EventRTCVADStatus` | `types.RTCVADStatus` | `type: "rtc_vad_status"` |
| `EventWhiteboardUpdate` | `types.WhiteboardUpdate` | `type: "whiteboard_update"` |

Tool call / result は UI に表示しません。

## ブラウザへ送る message
| `type` | 主な項目 | 送信先 |
| --- | --- | --- |
| `message` | `role`, `text`, `response_id`, `final`, `source` | 全接続 broadcast |
| `speech_end` | `source`, `captured_at` | 全接続 broadcast |
| `rtc_vad_status` | `input_level`, `threshold`, `captured_at` | 全接続 broadcast |
| `whiteboard_update` | `content` | 全接続 broadcast |
| `webrtc.answer` | `sdp` | `RTCSignal.ClientID` で特定された接続だけに送信 |
| `webrtc.ice` | `candidate` | `RTCSignal.ClientID` で特定された接続だけに送信 |

## ブラウザから受ける message
| `type` | 主な項目 | 内部変換 |
| --- | --- | --- |
| `webrtc.offer` | `sdp` | `EventRTCSignal` |
| `webrtc.answer` | `sdp` | `EventRTCSignal` |
| `webrtc.ice` | `candidate` | `EventRTCSignal` |

## 参照元
- `internal/components/wschat/wschat.go`
- `internal/types/event.go`
- `internal/types/types.go`
