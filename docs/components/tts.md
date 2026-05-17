# tts component

## 概要
`tts` component は、assistant のテキスト出力を ElevenLabs の streaming TTS へ渡し、返ってきた PCM 音声を event graph に流す component です。現行実装は `response_id` ごとに 1 本のアクティブストリームだけを扱い、再生中断と完了通知も同じ `response_id` を軸に制御します。

## 目的
- assistant の逐次テキストを低遅延で音声化する。
- 音声 chunk を `rtc` component が扱える形で graph に流す。
- 発話切り替え時に、古い TTS stream を中断できるようにする。
- `conversation` component が再生完了を追跡できるようにする。

## 担当範囲
- ElevenLabs への HTTP streaming TTS リクエスト
- 進行中 TTS stream の cancel
- `response_id` ごとの単一アクティブストリーム制御
- PCM chunk の `EventRealtimeAudio` 化
- stream 完了時の `EventTTSEnd` 発行

## 担当しないこと
- assistant テキスト自体の生成
- 音声の再生制御 UI
- WebRTC への Opus 変換とブラウザ返送
- 複数 `response_id` の同時再生

## 入力 event
| EventKind | payload | 用途 |
| --- | --- | --- |
| `EventRealtimeOutput` | `types.OutputLine` | assistant の逐次テキストを受ける。`Role` が空または `assistant`、`Final == false`、`Text != ""` のものだけを音声化する。 |
| `EventTTSCancel` | `types.TTSCancel` | 進行中 stream を中断する。`ResponseID` が空なら現在の stream 全体、指定ありなら一致時だけ中断する。 |

## 出力 event
| EventKind | payload | 出力条件 |
| --- | --- | --- |
| `EventRealtimeAudio` | `types.OutputAudio` | ElevenLabs から受けた PCM chunk を base64 化して流す。 |
| `EventTTSEnd` | `types.TTSEvent` | stream が EOF まで完了したときに出す。`ResponseID`、推定音声長、音声受信開始時刻を含む。 |

## 主要構成
- `internal/components/tts/elevenlabs.go`
  - stage の入口とライフサイクルを持つ。
  - `streamTTS` が upstream / downstream channel、HTTP client、進行中 cancel 関数、現在の `response_id` を管理する。
- `internal/types/types.go`
  - `OutputLine`、`OutputAudio`、`TTSEvent`、`TTSCancel` の型を定義する。
- `internal/types/event.go`
  - `EventRealtimeOutput`、`EventRealtimeAudio`、`EventTTSEnd`、`EventTTSCancel` を定義する。

## データフロー
### assistant text が音声 chunk になるまで
1. 上流から `EventRealtimeOutput` を受ける。
2. `tts` は assistant 向け・非 final・非空文字の行だけを対象にする。
3. `startStream` が、新しい `response_id` の音声化開始前に既存 stream を cancel する。
4. `streamRequest` が `POST https://api.elevenlabs.io/v1/text-to-speech/{voice_id}/stream?output_format=pcm_24000` を送る。
5. ElevenLabs から返る PCM 24000Hz mono 16-bit little-endian 相当の byte stream を順次読む。
6. `readStream` が受信 chunk を base64 化し、`EventRealtimeAudio` として downstream に流す。
7. EOF まで読めたら、総 byte 数から秒数を計算し、`EventTTSEnd` を downstream に流す。

### cancel されるまで
1. `EventTTSCancel` を受ける。
2. cancel payload の `ResponseID` が空なら、現在の stream を無条件で止める。
3. `ResponseID` が指定されている場合は、現在の `streamResponse` と一致するときだけ止める。
4. `cancelActiveStream` が context cancel を呼び、現在の `cancelStream` と `streamResponse` をクリアする。
5. 中断時は `readStream` / `streamRequest` 側が `ctx.Done()` を検知して終了する。

## `response_id` ごとの制御
- 同時に保持できるアクティブ stream は 1 本だけです。
- `startStream` は、同じ `response_id` がすでに再生中なら新しい request を作りません。
- 別の `response_id` の text を受けると、先に既存 stream を cancel してから新しい stream を開始します。
- `finishStream` は、自分が終了した stream の `response_id` がまだ現行値と一致している場合だけ state をクリアします。
- `EventTTSEnd` には `ResponseID` が入るため、上流はどの assistant 発話の再生完了かを対応付けできます。

## RTC への音声受け渡し
- `tts` が `rtc` に渡す音声 event は `EventRealtimeAudio` のみです。
- payload は `types.OutputAudio{Role: "assistant", Audio: <base64 PCM>}` です。
- `tts` 自体は PCM の再サンプルや Opus 変換をしません。
- `rtc` 側で base64 PCM を int16 に戻し、24000Hz から 48000Hz へアップサンプリングして WebRTC 下りトラックへ流します。
- `EventTTSCancel` は `tts` の stream 中断と、`rtc` の再生バッファ破棄の両方に使われます。

## 外部依存
- ElevenLabs Text-to-Speech API
  - `voice_id` を path parameter に持つ streaming endpoint を使う。
  - header として `xi-api-key`、body として `text`、`model_id`、`language_code`、`voice_settings` を送る。
- Go 標準ライブラリ
  - `net/http` で streaming response を読み、`context` で cancel を伝播する。

## 設定値
| 項目 | 内容 |
| --- | --- |
| `APIKey` | ElevenLabs API key。必須。 |
| `Voice` | ElevenLabs の voice ID。必須。 |
| `Model` | TTS model ID。未指定時は `eleven_v3`。 |
| `VoiceSettings` | request ごとの voice settings 上書き。未指定時は component 内のデフォルト値を使う。 |

## 現状の前提と制約
- 実装は HTTP streaming を使っており、WebSocket TTS は使っていません。
- `OutputAudio` には `ResponseID` が含まれないため、音声 chunk 単位でどの応答に属するかは event payload だけでは識別できません。
- `tts` は入力 text をそのまま 1 request に乗せるだけで、長文分割や `previous_text` / `next_text` のような連続性補助パラメータは使っていません。
- `language_code` は固定で `ja` を送っています。
- `readStream` が `EventTTSEnd` を出すのは EOF まで完了したときだけです。cancel 時や HTTP error 時は出しません。
- `voice_settings` のデフォルト値と `eleven_v3` 向け stability 正規化は component 内で独自に持っています。

## 不明点
- `response_id` 単位で複数 stream を並列保持する再設計が必要かどうかは、参照実装だけでは不明です。
- `EventRealtimeAudio` に `ResponseID` を含めるべきかどうかは、参照実装だけでは不明です。
- ElevenLabs 側で `pcm_24000` が契約上どのプラン条件で利用可能かは、確認した参照範囲だけでは不明です。

## 参照元
- `internal/components/tts/elevenlabs.go`
- `internal/components/rtc/output.go`
- `internal/types/event.go`
- `internal/types/types.go`
- `git show HEAD^:docs/5.音声パイプライン.md`
- ElevenLabs API reference: Stream speech
  - https://elevenlabs.io/docs/api-reference/text-to-speech/stream
- ElevenLabs Documentation overview
  - https://elevenlabs.io/docs/overview/intro
