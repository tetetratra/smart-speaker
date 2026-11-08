# 共通処理メモ

## event graph

`internal/graph` は stage 間の接続と event 転送だけを担当します。
業務判断、世代判定、履歴保存、tool result の扱いは各 component 側に閉じます。
接続は `cmd/smart-speaker/main.go` の `wireGraph` で、event の種類ごとに張ります。全体の流れは [アーキテクチャ](architecture.md) を参照してください。

## 主要 event

- `EventRTCSignal`: wschat と rtcpeer の間でやり取りする WebRTC signaling（offer / answer / ICE、音量）。
- `EventRTCPeerAudioFrame`: rtcpeer が decode した上り音声の PCM frame。wakegate を経由して realtime へ渡る。
- `EventRTCPeerOutputSink`: rtcpeer が通知する WebRTC 下り音声の送信先。rtcout と、wakegate 経由で realtime が受け取る。
- `EventLocalWakeUtterance`: 呼びかけ必須モード中に、wakegate がローカルSTTで呼びかけと判定した発話のテキスト。
- `EventLLMRequest`: realtime 内部で扱うテキスト入力（呼びかけと判定された発話、予約ジョブ）。他の component からは流れてこない。
- `EventGenerationChanged`: realtime が新しい世代を始めた通知。後段は古い世代の event を捨てる。
- `EventSpeechStart` / `EventSpeechEnd`: Realtime API の VAD による発話開始・終了。wschat が UI へ通知する。
- `EventRealtimeOutput`: UI 表示用の user / agent メッセージ。
- `EventRealtimeStatus`: Realtime API との接続状態。
- `EventResponseText`: realtime が受け取った応答テキストの差分。speechsegment が読み上げ単位に分割する。
- `EventTimelineItem`: speechsegment が作る読み上げ単位（`speech` item）。
- `EventAgentTimelineEnd`: 1つの応答の読み上げ単位をすべて出し終えた印。
- `EventPlayableSpeech`: tts が音声化した読み上げ単位。
- `EventAudioFrame`: audioframe が 20ms 単位に分割した音声フレーム。audiopace を経て rtcout へ渡る。
- `EventAudioSent`: rtcout が送信し終えたフレームの通知。realtime は割り込み時の履歴補正に、wschat は表示に使う。
- `EventToolRequest`: realtime から toolcaller へ渡すツール実行要求。
- `EventConversationCommitRequest`: toolcaller から realtime へ戻すツール実行結果。
- `EventWhiteboardUpdate`: toolcaller（`set_whiteboard`）から wschat へ渡すホワイトボード更新。
- `EventSessionReset`: realtime が無操作でセッションをリセットした通知。wschat が UI へ `session_reset` を送る。

## 共有Store

Store は graph node ではなく、必要な component へ依存注入する。

- `internal/states/generation` は最新の世代idを保持する。realtime が `generationcontroller` を通して進め、後段の component は `generationfilter.Attach` で受信直後に現在の世代かを判定する。
- `internal/states/conversationhistory` は会話履歴を保持する。realtime が記録し、再接続時の文脈の復元と、会話終了時のメモリ抽出に使う。
- `internal/states/memory` は長期メモリを JSON ファイルに保持する。
- `internal/states/wakemode` は呼びかけ必須モードのオン・オフと判定の設定を保持する。
- `internal/states/toolhistory` はツール呼び出し履歴の最新200件をメモリ上に保持する。
- `internal/states/userlocation` は現在地API（`/api/location`）で受け取った最後の場所をメモリ上に保持する。

## セッションリセット

- realtime が定期処理（`tick`）で、最後の発話から `conversation.idle_timeout_seconds` 秒たったかを判定する。応答中・ツール実行中・未送信の音声がある間はリセットしない。
- リセット時は会話履歴を取り出して空にし、取り出した履歴でメモリ抽出を起動して、Realtime API との接続を張り直す。
- `conversation.idle_timeout_seconds: 0` の場合、無操作リセットは無効になる。
- reset 用の graph event として `EventSessionReset` を発行し、`wschat` が UI へ `session_reset` を通知する。

## function calling

ツールは Realtime API の function calling で呼び出します。
realtime が API の function call を `EventToolRequest` として toolcaller へ渡し、toolcaller の実行結果を `function_call_output` として API へ返します。
成功後に再び応答を生成するかどうかは、ツールごとの応答方針（`tools.DefinitionWithResponsePolicy`）で決まります。
