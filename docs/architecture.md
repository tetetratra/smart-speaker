# Smart Speaker アーキテクチャ

この文書は `smart-speaker` の現在の会話 pipeline を説明する。
会話制御は旧 `conversation` component に集約せず、責務ごとの component と `internal/states/` 配下の共有Storeで構成する。

## 全体像

```mermaid
flowchart LR
  Browser["Browser / Web UI"] -->|/ws/chat| WS["wschat"]
  Browser <-->|WebRTC 音声| RTC["rtc"]

  WS -->|EventRTCSignal| RTC
  RTC -->|EventRTCSignal / VAD状態| WS
  RTC -->|EventHumanUtterance| UB["utterancebuffer"]
  UB -->|user commit| COMMIT["conversationcommitter"]

  GSTORE[("generation Store")]
  HSTORE[("conversation history Store")]

  UB -.->|Next| GSTORE
  COMMIT -.->|Append| HSTORE
  HSTORE -.->|Snapshot| LLM["llm"]

  COMMIT -->|EventLLMRequest| LLM
  COMMIT -->|EventRealtimeOutput| WS
  LLM -->|EventTimelineItem| GF1["generationfilter"]
  GF1 --> TTS["tts"]
  TTS -->|EventPlayableSpeech / EventTimelineItem| GF2["generationfilter"]
  GF2 --> SCH["scheduler"]
  SCH -->|EventScheduledItem| GF3["generationfilter"]
  GF3 --> ROUTER["router"]
  ROUTER -->|EventRealtimeAudio| RTC
  ROUTER -->|assistant commit| COMMIT
  ROUTER -->|EventToolRequest| TOOL["toolcaller"]
  TOOL -.->|CommitToolResult API| COMMIT
  TOOL -->|tool内部event| WS
```

## 主要な責務

- `utterancebuffer` は STT 由来の文字起こしを短時間バッファし、1つの user 発話にまとめて世代idを進める。
- `conversationcommitter` は user / assistant / tool result を会話履歴Storeへ保存し、保存後に LLM や UI へ振り分ける。
- `llm` は会話履歴Storeの snapshot を使って OpenAI Responses API を呼び、Structured Outputs の JSON timeline を `speech` / `wait` / `tool` として検証する。
- `generationfilter` は世代id付き event のうち最新世代だけを下流へ通す。
- `tts` は `speech` item を ElevenLabs で音声化し、`wait` / `tool` item は順序維持のためそのまま通す。
- `scheduler` は `speech` / `wait` / `tool` を同じ timeline として扱い、speech の再生時間や wait 秒数に従って次 item へ進む。
- `router` は実行タイミングが来た item を PLAY、会話コミッター、toolcaller へ振り分ける。
- `toolcaller` は local tool を実行し、結果を downstream event ではなく `CommitToolResult` API で会話コミッターへ戻す。

## Tool 呼び出し

OpenAI Responses API の function calling は使わない。
LLM には `{"items":[{"type":"tool","name":"...","args":{...}}]}` 形式の JSON timeline を出力させる。
tool は1回の LLM 応答の末尾に最大1件だけ許可し、tool の後に `speech` / `wait` / `tool` が続いた場合は契約違反として LLM component が最大10回 retry する。
10回失敗した場合はログに出して、その応答は捨てる。

`web_search` もこの local tool 経路で扱う。LLM は `web_search` を JSON timeline の `tool` item として呼び出し、`toolcaller` が local handler を実行する。handler 内部では OpenAI Responses API の hosted `web_search` を別 request で使うが、会話 pipeline 上は通常の local tool result と同じく `conversationcommitter` へ戻る。引数は `query` のみ、戻り値は `result` のみとする。

## 世代と履歴

世代idは `internal/states/generation` が保持する。
新しい確定 user 発話ごとに世代idを単調増加させ、古い LLM chunk や古い scheduler item は generationfilter で落とす。

会話履歴は `internal/states/conversationhistory` が保持する。
LLM request は必ず保存済みの履歴 snapshot から作る。
古い世代の tool result は実行済みの事実として保存し、`stale` metadata を付ける。

## 参照元

- `cmd/smart-speaker/main.go`
- `internal/graph/graph.go`
- `internal/types/event.go`
- `internal/types/timeline_item.go`
- `internal/states/generation/store.go`
- `internal/states/conversationhistory/store.go`
- `internal/components/utterancebuffer/stage.go`
- `internal/components/conversationcommitter/stage.go`
- `internal/components/llm/stage.go`
- `internal/components/generationfilter/stage.go`
- `internal/components/tts/elevenlabs.go`
- `internal/components/scheduler/stage.go`
- `internal/components/router/stage.go`
- `internal/components/toolcaller/toolcaller.go`
