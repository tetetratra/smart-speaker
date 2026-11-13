# smart-speaker

スマートフォンのブラウザを音声入力・音声再生の端末として使う、Web ベースのスマートスピーカーです。

ブラウザのマイク音声を WebRTC で Go サーバーへ送り、サーバーが OpenAI の Realtime API と会話します。
返答のテキストは VOICEVOX（または ElevenLabs）で音声にして、WebRTC でブラウザへ返して再生します。

## 主な機能

- 音声での会話。話しはじめると、読み上げ中の返答を止めて聞き取ります
- ホワイトボード：調べた結果や補足を画面に書き出します
- 外部サービスの操作：Google Calendar の予定の確認・登録、SwitchBot での家電操作、Web 検索
- 長期メモリ：会話から覚えておくことを抽出し、次の会話で使います。「覚えて」「忘れて」と頼むか、AIの提案に了承すると、その場で追加・変更・削除します（画面からも確認・編集できます）
- 予約ジョブ：「10分後に起こして」「平日8時に〜して」などと会話から登録できます
- 呼びかけ必須モード：「起きて」などの呼びかけで始まる発話だけに反応します
- 手元の Mac の Claude Code に、調べものや文章作成などの作業を頼めます

## すぐに起動する

```sh
cp config.sample.yaml config.yaml
export OPENAI_API_KEY=...
docker compose up --build
```

`http://localhost:5173` を開きます。
設定や外部サービスとの連携は、下の手順書を参照してください。

## 手順書

- [セットアップとローカル起動](docs/setup.md)：設定、起動、テスト、外部サービス連携の設定
- [本番環境へのデプロイ](docs/deploy.md)
- [agent-bridge](services/agent-bridge/README.md)：Claude Code 連携のために Mac で常駐させるブリッジ

設定の全項目と既定値は [config.sample.yaml](config.sample.yaml) にあります。

## ディレクトリ構成

| パス | 内容 |
|---|---|
| `cmd/smart-speaker` | サーバーの起動処理と HTTP API |
| `cmd/local-verify-realtime` | 外部 API を使わずに動かす確認用のサーバー |
| `internal/` | 会話処理（`components/realtime` など）、ツール（`tools/`）、状態の保存（`states/`） |
| `web/` | ブラウザの画面（React） |
| `services/local-stt` | 呼びかけ判定に使うローカルの文字起こし（faster-whisper） |
| `services/agent-bridge` | Claude Code 連携のブリッジ |
| `scripts/` | デプロイなどのスクリプト |

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
