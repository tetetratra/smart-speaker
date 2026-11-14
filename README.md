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

## コンポーネントとデータの流れ

細い実線はイベントと Inbound、太線は外部との通信、点線はストアの読み書きを表します。

### 音声の流れ

```mermaid
flowchart LR
    browser{{"ブラウザ<br/>web/"}}
    chat["wschat<br/>画面連携"]
    localStt{{"local-stt<br/>ローカル文字起こし"}}
    rt["realtime<br/>Realtime連携"]
    openaiRealtime{{"OpenAI Realtime API<br/>wss://api.openai.com/v1/realtime"}}
    elevenLabs{{"ElevenLabs API<br/>https://api.elevenlabs.io"}}
    voicevox{{"VOICEVOX<br/>http://voicevox:50021"}}

    subgraph g0["音声入力"]
        peer["rtcpeer<br/>WebRTC接続"]
        wake["wakegate<br/>呼びかけ判定"]
    end

    subgraph g1["音声出力"]
        segment["speechsegment<br/>発話区切り"]
        ttsNode["tts<br/>音声合成"]
        frame["audioframe<br/>音声フレーム分割"]
        pace["audiopace<br/>送信時刻調整"]
        output["rtcout<br/>WebRTC音声送信"]
    end

    subgraph g2["ストア"]
        generationStore[("generation<br/>応答世代")]
        wakeModeStore[("wakemode<br/>呼びかけモード")]
        ttsSelectionStore[("tts.Selector<br/>使う音声合成")]
    end

    browser ==>|"WebRTC<br/>マイク音声"| peer
    browser <==>|"WebSocket<br/>画面・WebRTC制御"| chat
    output ==>|"WebRTC<br/>応答音声"| browser
    wake <==>|"HTTP<br/>文字起こし"| localStt
    rt <==>|"WebSocket<br/>会話セッション"| openaiRealtime
    chat -->|"RTCSignal<br/>WebRTC制御"| peer
    peer -->|"RTCSignal<br/>WebRTC制御"| chat
    chat -->|"RTCSignal<br/>WebRTC制御"| output
    peer -->|"RTCPeerAudioFrame<br/>マイク音声フレーム<br/>RTCPeerOutputSink<br/>音声送信先"| wake
    peer -->|"RTCPeerOutputSink<br/>音声送信先"| output
    wake -->|"RTCPeerAudioFrame<br/>マイク音声フレーム<br/>RTCPeerOutputSink<br/>音声送信先<br/>LocalWakeUtterance<br/>呼びかけ判定済み発話"| rt
    wake -->|"WakeTranscription<br/>呼びかけ文字起こし結果"| chat
    rt -->|"ResponseText<br/>応答テキスト"| segment
    segment -->|"TimelineItem<br/>発話項目<br/>AgentTimelineEnd<br/>応答終了"| ttsNode
    ttsNode -->|"PlayableSpeech<br/>合成音声<br/>AgentTimelineEnd<br/>応答終了"| frame
    frame -->|"AudioFrame<br/>音声フレーム"| pace
    pace -->|"AudioFrame<br/>音声フレーム"| output
    output -->|"AudioSent<br/>音声送信済み"| rt
    output -->|"AudioSent<br/>音声送信済み"| chat
    rt -->|"GenerationChanged<br/>応答世代の変更<br/>RealtimeOutput<br/>会話内容の表示<br/>SpeechStart<br/>発話開始<br/>SpeechEnd<br/>発話終了<br/>RealtimeStatus<br/>Realtime接続状態<br/>SessionReset<br/>セッションリセット"| chat
    ttsNode <==>|"HTTPS<br/>音声合成"| elevenLabs
    ttsNode <==>|"HTTP<br/>音声合成"| voicevox
    rt <-.->|"世代の更新・判定"| generationStore
    generationStore -.->|"古い世代の破棄"| segment
    generationStore -.->|"古い世代の破棄"| ttsNode
    generationStore -.->|"古い世代の破棄"| frame
    generationStore -.->|"古い世代の破棄"| pace
    generationStore -.->|"古い世代の破棄"| output
    generationStore -.->|"古い世代の破棄"| chat
    wake <-.->|"呼びかけ判定"| wakeModeStore
    rt <-.->|"状態の参照、会話中の延長"| wakeModeStore
    ttsSelectionStore -.->|"使う音声合成"| ttsNode
    browser <-.->|"参照・切替"| ttsSelectionStore
```

### 会話

```mermaid
flowchart LR
    manager["conversationmanager<br/>会話管理"]
    openaiResponses{{"OpenAI Responses API<br/>https://api.openai.com/v1/responses"}}
    rt["realtime<br/>Realtime連携"]
    tool["toolcaller<br/>ツール実行"]
    scheduler("予約ジョブ実行<br/>automations")
    chat["wschat<br/>画面連携"]

    subgraph g0["ストア"]
        historyStore[("conversationhistory<br/>会話履歴")]
        memoryStore[("memory<br/>長期メモリ")]
        toolHistoryStore[("toolhistory<br/>ツール履歴")]
        automationStore[("automations<br/>予約ジョブ")]
    end

    manager <==>|"HTTPS<br/>メモリ抽出"| openaiResponses
    rt -->|"ConversationRecord<br/>会話記録<br/>ConversationContextAddition<br/>会話への情報追加<br/>ConversationResponseRequest<br/>会話への応答依頼<br/>ConversationHistorySync<br/>会話履歴の読み出し同期<br/>ConversationEnded<br/>会話終了<br/>ConversationRecordsUsed<br/>参照済みの外部記録"| manager
    manager --->|"ConversationContextAddition<br/>会話への情報追加<br/>ConversationResponseRequest<br/>会話への応答依頼<br/>ConversationHistorySync<br/>会話履歴の読み出し同期"| rt
    rt -->|"ToolRequest<br/>ツール実行依頼"| tool
    tool --->|"ConversationCommitRequest<br/>ツール結果の会話反映依頼"| rt
    scheduler -->|"Inbound<br/>予約ジョブの通知・Web検索の結果"| manager
    tool -->|"Inbound<br/>Web検索の結果"| manager
    manager <-.->|"記録、会話終了時に取り出し"| historyStore
    historyStore -.->|"接続時の文脈復元"| rt
    memoryStore -.->|"接続時の文脈"| rt
    manager <-.->|"会話終了時の抽出結果"| memoryStore
    tool -.->|"ツール実行の記録"| toolHistoryStore
    scheduler -.->|"ツール実行の記録"| toolHistoryStore
    rt -.->|"待機ツールの記録"| toolHistoryStore
    manager -.->|"外部からの差し込みの記録"| toolHistoryStore
    toolHistoryStore -.->|"履歴表示"| chat
    scheduler <-.->|"期限の来た予約の読み出し・次回時刻の保存"| automationStore
```

### ツール

```mermaid
flowchart LR
    tool["toolcaller<br/>ツール実行"]
    scheduler("予約ジョブ実行<br/>automations")
    manager["conversationmanager<br/>会話管理"]

    subgraph toolset["ツール"]
        webSearchTool["web_search<br/>Web検索"]
        claudeCodeTool["ask_claude_code<br/>answer_claude_code_permission<br/>cancel_claude_code<br/>list_claude_code_sessions<br/>read_claude_code_session<br/>Claude Code連携"]
        calendarTool["google_calendar_list<br/>google_calendar_create<br/>google_calendar_update<br/>Googleカレンダー"]
        switchBotTool["hub2_get_temperature<br/>hub2_get_humidity<br/>hub2_get_light_level<br/>switchbot_execute_scene<br/>SwitchBot"]
        memoryTool["list_memories<br/>add_memory<br/>update_memory<br/>delete_memory<br/>長期メモリ"]
        wakeModeTool["get_wake_mode<br/>set_wake_mode<br/>呼びかけモード"]
        automationTool["register_job<br/>list_jobs<br/>update_job<br/>delete_job<br/>予約"]
        whiteboardTool["set_whiteboard<br/>ホワイトボード表示"]
    end


    openaiResponses{{"OpenAI Responses API<br/>https://api.openai.com/v1/responses"}}
    agentBridge{{"agent-bridge<br/>Claude Code連携（Mac）"}}
    googleCalendar{{"Google Calendar API<br/>https://www.googleapis.com/calendar/v3"}}
    googleOAuth{{"Google OAuth<br/>カレンダーの認可"}}
    googleTokenStore[("google_oauth_token.json<br/>Googleの認証トークン")]
    switchBot{{"SwitchBot API<br/>https://api.switch-bot.com"}}
    memoryStore[("memory<br/>長期メモリ")]
    wakeModeStore[("wakemode<br/>呼びかけモード")]
    automationStore[("automations<br/>予約ジョブ")]
    locationStore[("userlocation<br/>ユーザーの居場所")]

    tool -->|"ツール呼び出し"| toolset
    scheduler -->|"スクリプトからのツール呼び出し"| toolset

    webSearchTool <==>|"HTTPS<br/>Web検索"| openaiResponses
    claudeCodeTool <==>|"HTTP<br/>依頼・承認・取消・参照"| agentBridge
    agentBridge -->|"HTTP・Inbound<br/>Claude Codeの通知"| manager
    calendarTool <==>|"HTTPS<br/>予定の参照・作成・更新"| googleCalendar
    calendarTool <-.->|"読み出し・更新後の保存"| googleTokenStore
    calendarTool <==>|"HTTPS<br/>トークンの更新"| googleOAuth
    switchBotTool <==>|"HTTPS<br/>計測値の取得・シーン実行"| switchBot
    memoryTool <-.->|"一覧・追加・更新・削除"| memoryStore
    wakeModeTool <-.->|"参照・切替"| wakeModeStore
    automationTool <-.->|"登録・一覧・更新・削除"| automationStore
    %% 居場所はツールにする予定で、まだどこからも使われていない。位置だけをそろえる見えない線。
    whiteboardTool ~~~ locationStore
```
