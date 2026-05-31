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
- `EventRealtimeAudio`: rtcout 再生用の agent 音声。
- `EventSessionReset`: idle timeout によるセッション reset 通知。
- `EventAgentTimelineEnd`: LLM が当該 generation の timeline item をすべて発行し終えた印。
- `EventAgentSpeechPlaybackEnd`: scheduler が当該 generation の timeline（speech 再生待ち・wait・tool）を完了した印。`wschat` が `agent_speech_end` として UI へ通知する。

## 共有Store

- `internal/states/generation` は最新世代idを保持する。
- `internal/states/conversationhistory` は LLM に渡す会話履歴を保持する。
- `internal/states/agentstatus` は LLM がひとりごと候補判定で参照する `idle` / `active` 状態を保持する。
- `internal/states/playbackspeed` はエージェント発話の再生速度倍率（`1` / `1.5` / `2` / `3`）を保持する。`wschat` が WebSocket 経由で更新し、`tts` が ElevenLabs の `voice_settings.speed` 用に、`scheduler` が wait 秒数調整用に読み取る。永続化は行わない。
- Store は graph node ではなく、必要な component へ依存注入する。
- `sessionreset` は idle timeout 到達時に `conversationhistory.Store.Reset()`、`generation.Store.Next()`、`agentstatus.Store.SetIdle()` を呼び、履歴クリア、古い世代の抑止、ひとりごと候補判定の再有効化を同時に行う。
- `sessionactivate` は `speech` timeline item 通過時に `agentstatus.Store.SetActive()` を呼び、LLM 応答が始まったセッションを active に戻す。

## セッションリセット

- `sessionreset` は `utterancebuffer` から出る user の `EventConversationCommitRequest` を横付けで監視する。
- `CONVERSATION_IDLE_TIMEOUT_SECONDS` 秒だけ user 発話がない場合、登録済み hook の `Exec(ctx)` を同期実行し、会話履歴を空にして世代idを前進させ、agent status を `idle` にする。
- `CONVERSATION_IDLE_TIMEOUT_SECONDS=0` の場合、idle reset は無効になる。
- reset 用の graph event として `EventSessionReset` を発行し、`wschat` が UI へ `session_reset` を通知する。

## function calling

OpenAI function calling 用の event と payload は削除済みです。
tool 呼び出しは LLM が Structured Outputs の JSON timeline 内の `tool` item として出力し、scheduler / router を通って `toolcaller` へ到達します。
1 応答に複数の tool item を出せます。各 tool 定義の `x_tool_mode` が `write` の場合、成功結果は `toolcaller` が会話履歴へ commit せず LLM への再投入も行いません。read 系 tool の成功結果と write 系 tool のエラー結果だけが `EventConversationCommitRequest` として `conversationcommitter` へ渡ります。
