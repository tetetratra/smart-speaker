# conversation pipeline

旧 `conversation` component は削除済みです。
現在の会話制御は、複数 component と共有Storeで構成します。

## 構成

- `utterancebuffer`: STT の確定テキストを発話単位にまとめる。
- `conversationcommitter`: 会話履歴Storeへ保存してから LLM / UI / TTS 系へ振り分ける。
- `llm`: OpenAI Responses API を呼び、NDJSON timeline 契約を検証する。
- `generationfilter`: 最新世代の event だけ通す。
- `tts`: speech item を音声化する。
- `scheduler`: speech / wait / tool の実行順序を管理する。
- `router`: 再生、assistant commit、tool 実行へ振り分ける。

## 共有Store

- `internal/states/generation`: 最新世代idを保持する。
- `internal/states/conversationhistory`: user / assistant / tool の履歴を保持する。

## 削除された旧責務

- 旧 `conversation` 内の Rule / Effect / runtime loop は削除済み。
- `EventTTSCancel` による明示的な割り込みは削除済み。
- OpenAI function calling の tool result 再投入は削除済み。
