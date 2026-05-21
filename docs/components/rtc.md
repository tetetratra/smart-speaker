# rtc component

`rtc` component は WebRTC 接続を終端し、ブラウザ音声の入力と assistant 音声の返送を担当します。

## 入力

- `EventRTCSignal`
  - WebRTC signaling を処理する。
- `EventRealtimeAudio`
  - router から渡された assistant 音声を WebRTC 下りトラックへ流す。

## 出力

- `EventRTCSignal`
  - ブラウザへ返す signaling 応答。
- `EventHumanUtterance`
  - Google Speech-to-Text v2 の final transcript。
- `EventSpeechEnd`
  - サーバー側 VAD の発話終了。
- `EventRTCVADStatus`
  - VAD の入力レベルとしきい値。

## 方針

- `rtc` は assistant 応答生成や TTS API 呼び出しを担当しない。
- PLAY に渡った音声は止めない。
- `EventTTSCancel` による再生バッファ破棄は行わない。
