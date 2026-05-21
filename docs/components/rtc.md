# rtc component

## 1. 目的
- ブラウザとの WebRTC 接続を終端し、音声入力と音声出力返送を担当する。
- サーバー側で VAD と STT を実行し、音声入力を `EventHumanUtterance` に変換して event graph へ流す。
- assistant 音声を WebRTC 下り音声トラックとしてブラウザへ返す。

## 2. 担当範囲
- WebRTC signaling
- 音声入力
- VAD
- STT
- 音声出力返送
- ブラウザとの境界

## 3. 責務と境界
### rtc が担当すること
- `webrtc.offer` / `webrtc.answer` / `webrtc.ice` を処理し、peer ごとの `PeerConnection` を管理する。
- ブラウザから届く Opus 音声を PCM に復号し、モノラル化して VAD と STT に渡す。
- サーバー側 VAD により `EventSpeechEnd` / `EventRTCVADStatus` を発行する。
- Google Speech-to-Text v2 の final transcript を `EventHumanUtterance` として発行する。
- `EventRealtimeAudio` で受けた PCM 音声を 48kHz Opus に変換し、各 peer の下りトラックへ返す。

### rtc が担当しないこと
- assistant 応答テキストの生成
- TTS API 呼び出しそのもの
- WebSocket endpoint の提供
- ブラウザ UI 上の描画と再生 UI 制御

## 4. 主要構成
- `internal/components/rtc/rtc.go`
  - stage の入口。
  - VAD、STT、Opus まわりの定数と stage ライフサイクルを持つ。
- `internal/components/rtc/signaling.go`
  - signaling 処理と `peerState` 管理を持つ。
  - `PeerConnection`、下り音声トラック、Opus encoder、pending ICE、active speaker 制御を扱う。
- `internal/components/rtc/input.go`
  - 上り音声の受信処理を持つ。
  - Opus 復号、モノラル化、adaptive VAD、STT streaming、prebuffer 管理を行う。
- `internal/components/rtc/output.go`
  - 下り音声の返送処理を持つ。
  - PCM decode、48kHz 化、必要時の stereo 化、Opus encode、track 書き込みを行う。

## 5. データフロー
### 5.1 ブラウザ音声が確定発話になるまで
1. ブラウザが `/ws/chat` 経由で `webrtc.offer` と `webrtc.ice` を送る。
2. `rtc` が `PeerConnection` を作成し、下り音声用 track を追加して `webrtc.answer` を返す。
3. ブラウザの音声トラックが server に届くと、`handleIncomingTrack` が RTP payload を Opus 復号する。
4. 複数 channel の場合はモノラル化し、frame energy を計測する。
5. 直近 1 分の energy 履歴からしきい値を更新し、発話開始・終了を判定する。
6. 発話開始時に active speaker を確保し、直前 3 秒の prebuffer を付けて STT stream を開始する。
7. 発話中の PCM を Google Speech-to-Text v2 に送る。
8. final transcript が返ると、`Source=server-stt` の `EventHumanUtterance` を発行する。
9. 発話終了時は `EventSpeechEnd` を発行し、STT stream の `CloseSend` を行う。

### 5.2 assistant 音声がブラウザへ返るまで
1. 上流から `EventRealtimeAudio` を受ける。
2. `rtc` が base64 PCM を int16 に戻す。
3. 24000Hz PCM を 48000Hz へ 2 倍アップサンプリングする。
4. peer が stereo を要求している場合だけ左右複製する。
5. `sendLoop` が 20ms ごとに PCM を Opus encode し、下り track に書き込む。
6. ブラウザは返ってきた WebRTC 音声トラックを再生する。

## 6. ブラウザとの境界
### ブラウザから rtc に入るもの
- `EventRTCSignal`
  - `webrtc.offer`
  - `webrtc.answer`
  - `webrtc.ice`
- WebRTC 上り音声トラック

### rtc からブラウザへ返すもの
- `EventRTCSignal`
  - `webrtc.answer`
  - `webrtc.ice`
- `EventSpeechEnd`
- `EventRTCVADStatus`
- WebRTC 下り音声トラック

### 境界の実装位置
- WebSocket メッセージの JSON 入出力は `internal/components/wschat/wschat.go` が担当する。
- `rtc` は `EventRTCSignal` と WebRTC media を受け取り、event graph と Pion WebRTC の間を橋渡しする。

## 7. 主要なルール
- active speaker は 1 peer に制限される。別 peer が同時に話しても、その間は処理対象にならない。
- 発話開始判定は `200ms`、発話終了判定は `500ms`。
- VAD しきい値は直近 1 分の energy 履歴の中央値に `50` を足して計算し、下限は `50`。
- STT には final result だけを採用する。
- STT 停止の追加待ちは `0ms`。
- 下り音声の Opus 送信周期は `20ms`。
- ICE の UDP port range は `50000-50100`。

## 8. 外部依存
- Pion WebRTC
  - `PeerConnection`、track、ICE を扱う。
- Opus
  - 上り復号と下りエンコードを行う。
- Google Speech-to-Text v2
  - `StreamingRecognize` を使って server-side STT を行う。

## 9. 設定値
- `SpeechProjectID`
  - Google Cloud project ID。
- `SpeechRecognizer`
  - 未指定時は `"_"`。
- `SpeechLanguage`
  - 未指定時は `ja-JP`。
- `SpeechCredsJSON`
  - Google Speech client 作成時に使う認証 JSON。
- `IceHostIPs`
  - 指定時は host candidate 用の NAT 1:1 IP として使う。

## 10. 現状の前提と制約
- `GOOGLE_CLOUD_PROJECT` 相当の設定が空の場合、server-side STT は無効化される。
- VAD は Google の voice activity events ではなく、server 側の energy ベース実装に依存する。
- prebuffer はモノラル PCM を最大 3 秒保持する。
- TTS 音声の入力元は `EventRealtimeAudio` だけであり、TTS provider の詳細は rtc の責務外。

## 11. 不明点
- ブラウザ側でどの条件で `webrtc.answer` を送る経路が必要になるかは、この component の参照実装だけでは明確でない。
- STT の recognizer を `"_"` 以外でどう運用する想定かは、この component の参照実装だけでは明確でない。

## 12. 参照元
- `internal/components/rtc/rtc.go`
- `internal/components/rtc/signaling.go`
- `internal/components/rtc/input.go`
- `internal/components/rtc/output.go`
- `internal/components/rtc/input_test.go`
- `internal/components/wschat/wschat.go`
- `git show HEAD^:docs/5.音声パイプライン.md`
- `git show HEAD^:docs/1.全体アーキテクチャとイベントグラフ.md`
