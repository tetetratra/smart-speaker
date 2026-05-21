# tts component

`tts` component は LLM の timeline item のうち `speech` を ElevenLabs で音声化します。
`wait` / `tool` item は順序維持のため、そのまま下流へ通します。

## 入力

- `EventTimelineItem`
  - `speech`: ElevenLabs に渡して音声化する。
  - `wait`: そのまま downstream へ流す。
  - `tool`: そのまま downstream へ流す。

## 出力

- `EventPlayableSpeech`
  - payload は `types.PlayableSpeech`
  - base64 PCM 音声、本文、世代id、推定再生時間を含む。
- `EventTimelineItem`
  - `wait` / `tool` はそのまま通す。

## 方針

- TTS は scheduler の前に置く。
- scheduler は TTS が付与した `duration_seconds` を使って次 item へ進む。
- `EventTTSCancel` は使わない。
- PLAY に渡った音声は止めない。
