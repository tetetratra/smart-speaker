# コンポーネントとイベントの全体図

サーバーの `wireGraph` に登録された全11コンポーネントと、コンポーネント間で転送する全イベントを示します。
枠内は「コンポーネント名 → 日本語名」、矢印は「イベント名 → 日本語名」の2行表記です。
同じ送信元と送信先でも、イベントごとに矢印を分けています。

```mermaid
flowchart TB
    chat["wschat<br/>画面連携"]
    peer["rtcpeer<br/>WebRTC接続"]
    wake["wakegate<br/>呼びかけ判定"]
    rt["realtime<br/>Realtime連携"]
    manager["conversationmanager<br/>会話管理"]
    segment["speechsegment<br/>発話区切り"]
    ttsNode["tts<br/>音声合成"]
    frame["audioframe<br/>音声フレーム分割"]
    pace["audiopace<br/>送信時刻調整"]
    output["rtcout<br/>WebRTC音声送信"]
    tool["toolcaller<br/>ツール実行"]

    rt -->|"EventConversationRecord<br/>会話記録"| manager
    rt -->|"EventConversationContextAddition<br/>会話への情報追加"| manager
    rt -->|"EventConversationResponseRequest<br/>会話への応答依頼"| manager
    rt -->|"EventConversationHistorySync<br/>会話履歴の読み出し同期"| manager
    rt -->|"EventConversationEnded<br/>会話終了"| manager
    rt -->|"EventConversationRecordsUsed<br/>参照済みの外部記録"| manager
    manager -->|"EventConversationContextAddition<br/>会話への情報追加"| rt
    manager -->|"EventConversationResponseRequest<br/>会話への応答依頼"| rt
    manager -->|"EventConversationHistorySync<br/>会話履歴の読み出し同期"| rt
    peer -->|"EventRTCPeerOutputSink<br/>音声送信先"| output
    chat -->|"EventRTCSignal<br/>WebRTC制御"| output
    chat -->|"EventRTCSignal<br/>WebRTC制御"| peer
    peer -->|"EventRTCSignal<br/>WebRTC制御"| chat
    peer -->|"EventRTCPeerAudioFrame<br/>マイク音声フレーム"| wake
    peer -->|"EventRTCPeerOutputSink<br/>音声送信先"| wake
    wake -->|"EventRTCPeerAudioFrame<br/>マイク音声フレーム"| rt
    wake -->|"EventRTCPeerOutputSink<br/>音声送信先"| rt
    wake -->|"EventLocalWakeUtterance<br/>呼びかけ判定済み発話"| rt
    wake -->|"EventWakeTranscription<br/>呼びかけ文字起こし結果"| chat
    rt -->|"EventResponseText<br/>応答テキスト"| segment
    segment -->|"EventTimelineItem<br/>発話項目"| ttsNode
    segment -->|"EventAgentTimelineEnd<br/>応答終了"| ttsNode
    ttsNode -->|"EventPlayableSpeech<br/>合成音声"| frame
    ttsNode -->|"EventAgentTimelineEnd<br/>応答終了"| frame
    frame -->|"EventAudioFrame<br/>音声フレーム"| pace
    pace -->|"EventAudioFrame<br/>音声フレーム"| output
    output -->|"EventAudioSent<br/>音声送信済み"| chat
    output -->|"EventAudioSent<br/>音声送信済み"| rt
    rt -->|"EventGenerationChanged<br/>応答世代の変更"| chat
    rt -->|"EventRealtimeOutput<br/>会話内容の表示"| chat
    rt -->|"EventSpeechStart<br/>発話開始"| chat
    rt -->|"EventSpeechEnd<br/>発話終了"| chat
    rt -->|"EventRealtimeStatus<br/>Realtime接続状態"| chat
    rt -->|"EventSessionReset<br/>セッションリセット"| chat
    rt -->|"EventToolRequest<br/>ツール実行依頼"| tool
    tool -->|"EventConversationCommitRequest<br/>ツール結果の会話反映依頼"| rt
```

矢印はイベントの転送方向を表します。
条件付きで発行されるイベントも含めた接続定義なので、1回の会話ですべての矢印を通るとは限りません。
`EventAudioSent` は音声トラックへの書き込み成功を表し、ブラウザでの再生完了を表すものではありません。
`EventConversationHistorySync` は履歴を読み出す順序を揃えるためのイベントで、履歴本体は運びません。

この図の範囲は `graph.Stage` 間のイベント接続です。
外部APIへの通信、共有ストアの読み書き、HTTPハンドラーや予約ジョブからの入力は、グラフ外の処理です。

## 実装の参照先

- [コンポーネントの生成と接続定義（buildStages / wireGraph）](../cmd/smart-speaker/main.go)
- [イベントの転送処理（Graph）](../internal/graph/graph.go)
- [共通イベントとツール実行依頼](../internal/types/event.go)
- [Realtime、音声、呼びかけ関連のイベント](../internal/types/realtime.go)
- [会話管理のイベント](../internal/types/conversation_events.go)
- [音声送信と送信済みイベントの発行](../internal/components/rtcout/stage.go)
