# 共通処理メモ

## event graph

`internal/graph` は stage 間の接続と event 転送だけを担当します。
業務判断、世代判定、履歴保存、tool result の扱いは各 component 側に閉じます。

## 主要 event

- `EventHumanUtterance`: STT の確定テキスト。
- `EventConversationCommitRequest`: user / agent / tool_call / tool_result の履歴保存要求。
- `EventLLMRequest`: LLM component への推論要求。
- `EventTimelineItem`: LLM が返す `speech` / `wait` / `tool` item。
- `EventPlayableSpeech`: TTS 済みの speech item。
- `EventScheduledItem`: scheduler が実行タイミングに到達した item。
- `EventToolRequest`: router から toolcaller へ渡す tool 実行要求。
- `EventRealtimeOutput`: UI 表示用の user / agent message。
- `EventRealtimeAudio`: rtc 再生用の agent 音声。

## 共有Store

- `internal/states/generation` は最新世代idを保持する。
- `internal/states/conversationhistory` は LLM に渡す会話履歴を保持する。
- Store は graph node ではなく、必要な component へ依存注入する。

## function calling

OpenAI function calling 用の event と payload は削除済みです。
tool 呼び出しは LLM が Structured Outputs の JSON timeline 内の `tool` item として出力し、scheduler / router を通って `toolcaller` へ到達します。
