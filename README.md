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
    browser{{"ブラウザ<br/>web/"}}

    chat["wschat<br/>画面連携"]

    subgraph input["音声入力"]
        peer["rtcpeer<br/>WebRTC接続"]
        wake["wakegate<br/>呼びかけ判定"]
    end
    localStt{{"local-stt<br/>ローカル文字起こし"}}

    subgraph conversation["会話"]
        manager["conversationmanager<br/>会話管理"]
        rt["realtime<br/>Realtime連携"]
        tool["toolcaller<br/>ツール実行"]
    end
    openaiRealtime{{"OpenAI Realtime API<br/>wss://api.openai.com/v1/realtime"}}
    openaiResponses{{"OpenAI Responses API<br/>https://api.openai.com/v1/responses"}}

    subgraph outputGroup["音声出力"]
        direction LR
        segment["speechsegment<br/>発話区切り"]
        ttsNode["tts<br/>音声合成"]
        frame["audioframe<br/>音声フレーム分割"]
        pace["audiopace<br/>送信時刻調整"]
        output["rtcout<br/>WebRTC音声送信"]
    end

    subgraph stores["ストア"]
        historyStore[("conversationhistory<br/>会話履歴")]
        memoryStore[("memory<br/>長期メモリ")]
        toolHistoryStore[("toolhistory<br/>ツール履歴")]
        generationStore[("generation<br/>応答世代<br/>出力側で古い世代を破棄")]
        wakeModeStore[("wakemode<br/>呼びかけモード")]
    end

    browser ==>|"WebRTC<br/>マイク音声"| peer
    browser <==>|"WebSocket<br/>画面・WebRTC制御"| chat
    output ==>|"WebRTC<br/>応答音声"| browser
    wake <==>|"HTTP<br/>文字起こし"| localStt
    rt <==>|"WebSocket<br/>会話セッション"| openaiRealtime
    manager <==>|"HTTPS<br/>メモリ抽出"| openaiResponses
    tool <==>|"HTTPS<br/>Web検索"| openaiResponses

    chat -->|"RTCSignal"| peer
    peer -->|"RTCSignal"| chat
    chat -->|"RTCSignal"| output
    peer -->|"RTCPeerAudioFrame<br/>RTCPeerOutputSink"| wake
    peer -->|"RTCPeerOutputSink"| output
    wake -->|"RTCPeerAudioFrame<br/>RTCPeerOutputSink<br/>LocalWakeUtterance"| rt
    wake -->|"WakeTranscription"| chat
    rt -->|"ConversationRecord<br/>ConversationContextAddition<br/>ConversationResponseRequest<br/>ConversationHistorySync<br/>ConversationEnded<br/>ConversationRecordsUsed"| manager
    manager --->|"ConversationContextAddition<br/>ConversationResponseRequest<br/>ConversationHistorySync"| rt
    rt -->|"ToolRequest"| tool
    tool --->|"ConversationCommitRequest"| rt
    rt -->|"ResponseText"| segment
    segment -->|"TimelineItem<br/>AgentTimelineEnd"| ttsNode
    ttsNode -->|"PlayableSpeech<br/>AgentTimelineEnd"| frame
    frame -->|"AudioFrame"| pace
    pace -->|"AudioFrame"| output
    output -->|"AudioSent"| rt
    output -->|"AudioSent"| chat
    rt -->|"GenerationChanged<br/>RealtimeOutput<br/>SpeechStart<br/>SpeechEnd<br/>RealtimeStatus<br/>SessionReset"| chat

    manager <-.->|"記録・取り出し"| historyStore
    historyStore -.->|"文脈の復元"| rt
    memoryStore -.->|"文脈"| rt
    manager -.->|"抽出結果"| memoryStore
    tool <-.->|"メモリツール"| memoryStore
    tool -.-> toolHistoryStore
    rt -.-> toolHistoryStore
    manager -.-> toolHistoryStore
    toolHistoryStore -.->|"履歴表示"| chat
    rt <-.->|"更新・判定"| generationStore
    wake <-.-> wakeModeStore
    rt <-.-> wakeModeStore
    tool <-.->|"切替ツール"| wakeModeStore
```
