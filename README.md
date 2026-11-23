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
| `internal/plugin`、`internal/plugins/<名前>` | プラグインの受け口と、各プラグイン（ツール・HTTP・状態などを1つのディレクトリにまとめた機能） |
| `web/` | ブラウザの画面（React） |
| `services/local-stt` | 呼びかけ判定に使うローカルの文字起こし（faster-whisper） |
| `services/agent-bridge` | Claude Code 連携のブリッジ |
| `scripts/` | デプロイなどのスクリプト |

## コンポーネントとデータの流れ

細い実線はイベントと、gateway とグラフの外との受け渡し（Deliver）、太線は外部との通信、点線はストアの読み書きを表します。
名前の上の【】は、その部品が属するプラグインです。

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
    chat -->|"EventRTCSignal<br/>WebRTC制御"| peer
    peer -->|"EventRTCSignal<br/>WebRTC制御"| chat
    peer -->|"EventRTCPeerAudioFrame<br/>マイク音声フレーム<br/>EventRTCPeerOutputSink<br/>音声送信先"| wake
    peer -->|"EventRTCPeerOutputSink<br/>音声送信先"| output
    wake -->|"EventRTCPeerAudioFrame<br/>マイク音声フレーム<br/>EventRTCPeerOutputSink<br/>音声送信先<br/>EventLocalWakeUtterance<br/>呼びかけ判定済み発話"| rt
    wake -->|"EventWakeTranscription<br/>呼びかけ文字起こし結果"| chat
    rt -->|"EventResponseText<br/>応答テキスト"| segment
    segment -->|"EventTimelineItem<br/>発話項目<br/>EventAgentTimelineEnd<br/>応答終了"| ttsNode
    ttsNode -->|"EventPlayableSpeech<br/>合成音声<br/>EventAgentTimelineEnd<br/>応答終了"| frame
    frame -->|"EventAudioFrame<br/>音声フレーム"| pace
    pace -->|"EventAudioFrame<br/>音声フレーム"| output
    output -->|"EventAudioSent<br/>音声送信済み"| rt
    output -->|"EventAudioSent<br/>音声送信済み"| chat
    rt -->|"EventGenerationChanged<br/>応答世代の変更<br/>EventSpeechStart<br/>発話開始<br/>EventSpeechEnd<br/>発話終了<br/>EventRealtimeStatus<br/>Realtime接続状態<br/>SessionReset<br/>セッションリセット"| chat
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
    webSearchTool("【Web検索】<br/>web_search")
    scheduler["automation<br/>予約ジョブ"]
    runner("script.Runner<br/>スクリプト実行")
    gateway["gateway<br/>グラフの外との境目"]
    locationAPI("【現在地】<br/>位置情報API<br/>POST /api/location")
    hub2Watcher("【SwitchBot】<br/>Hub 2 の監視<br/>起動時と1分おき")
    switchBot{{"SwitchBot API<br/>https://api.switch-bot.com"}}
    chat["wschat<br/>画面連携"]
    browser{{"ブラウザ<br/>web/"}}

    subgraph g0["ストア"]
        historyStore[("conversationhistory<br/>会話履歴")]
        memoryStore[("memory<br/>長期メモリ")]
        automationStore[("automations<br/>予約ジョブ")]
    end

    manager <==>|"HTTPS<br/>メモリ抽出"| openaiResponses
    rt -->|"EventConversationInput{ModelAlreadyKnows: true}<br/>ユーザーの発話・ツールの記録<br/>EventConversationInput{ModelAlreadyKnows: true, UserAlreadyKnows: true}<br/>モデルの発話・遮られた記録などの記録<br/>EventConversationInput{ModelShouldRespond: true}<br/>ローカルの呼びかけ判定を通った発話<br/><br/>EventConversationHistorySync<br/>会話履歴の読み出し同期<br/>EventConversationIdle<br/>無操作の継続<br/>EventConversationRecordsUsed<br/>参照済みの外部記録"| manager
    manager --->|"EventConversationInput{}<br/>会話への情報追加<br/>EventConversationInput{ModelShouldRespond: true}<br/>会話への応答依頼・ローカルの呼びかけ判定を通った発話<br/><br/>EventConversationHistorySync<br/>会話履歴の読み出し同期"| rt
    rt -->|"EventToolRequest<br/>ツール実行依頼"| tool
    tool --->|"EventConversationCommitRequest<br/>ツール結果の会話反映依頼"| rt
    manager -->|"EventConversationInput{Subscribable: true}<br/>購読可能な記録（schedule.source が一致するジョブを動かす）<br/>※会話に入れた記録はすべて流れ、購読可能なものだけを使う"| scheduler
    scheduler -->|"ジョブのスクリプトを実行"| runner
    runner -->|"remind・ツールの呼び出しと結果・スクリプトの失敗"| scheduler
    scheduler -->|"EventConversationInput{ModelShouldRespond: true}<br/>remind<br/>EventConversationInput{}<br/>スクリプトから呼んだツールの記録・スクリプトの失敗"| manager
    tool -->|"呼び出し<br/>受け付けだけ返る"| webSearchTool
    runner -->|"スクリプトからの呼び出し"| webSearchTool
    webSearchTool -->|"Deliver<br/>後から届く検索結果"| gateway
    locationAPI -->|"Deliver<br/>居場所の変化"| gateway
    hub2Watcher <==>|"HTTPS<br/>温度・湿度・明るさの取得"| switchBot
    hub2Watcher -->|"Deliver<br/>温度・湿度・明るさの段階の変化"| gateway
    gateway -->|"EventConversationInput{}<br/>会話への情報追加<br/>EventConversationInput{ModelShouldRespond: true}<br/>会話への応答依頼<br/>EventConversationInput{Subscribable: true}<br/>居場所・温度・湿度・明るさの変化"| manager
    manager <-.->|"記録、無操作が続いたときに<br/>時間がたった・大きくなりすぎた古い会話を取り出し"| historyStore
    historyStore -.->|"接続時の文脈復元"| rt
    memoryStore -.->|"接続時の文脈"| rt
    manager <-.->|"古い会話からの抽出結果"| memoryStore
    manager -->|"EventConversationRecordAdded<br/>画面に出す記録の追加"| chat
    chat ==>|"WebSocket<br/>記録の追加<br/>（ユーザーの発話はチャット欄、それ以外はホワイトボード）"| browser
    scheduler <-.->|"期限の来た予約の読み出し・次回時刻の保存"| automationStore
```

### ツール

```mermaid
flowchart LR
    rt["realtime<br/>Realtime連携"]
    tool["toolcaller<br/>ツール実行"]
    scheduler["automation<br/>予約ジョブ"]
    runner("script.Runner<br/>スクリプト実行")
    gateway["gateway<br/>グラフの外との境目"]

    subgraph toolset["ツール"]
        webSearchTool["【Web検索】<br/>web_search"]
        claudeCodeTool["【Claude Code】<br/>ask_claude_code<br/>answer_claude_code_permission<br/>cancel_claude_code<br/>list_claude_code_sessions<br/>read_claude_code_session<br/>（agentbridge.Service）"]
        calendarTool["【Googleカレンダー】<br/>google_calendar_list<br/>google_calendar_create<br/>google_calendar_update"]
        switchBotTool["【SwitchBot】<br/>switchbot_execute_scene"]
        memoryTool["list_memories<br/>add_memory<br/>update_memory<br/>delete_memory<br/>長期メモリ"]
        wakeModeTool["get_wake_mode<br/>set_wake_mode<br/>呼びかけモード"]
        volumeTool["【再生音量】<br/>set_playback_volume"]
        automationTool["register_job<br/>list_jobs<br/>update_job<br/>delete_job<br/>予約"]
        whiteboardTool["set_whiteboard<br/>ホワイトボード表示"]
        locationTool["【現在地】<br/>get_current_location"]
    end


    openaiResponses{{"OpenAI Responses API<br/>https://api.openai.com/v1/responses"}}
    agentBridge{{"agent-bridge<br/>Claude Code連携（Mac）"}}
    googleCalendar{{"Google Calendar API<br/>https://www.googleapis.com/calendar/v3"}}
    googleOAuth{{"Google OAuth<br/>カレンダーの認可"}}
    googleTokenStore[("【Googleカレンダー】<br/>google_oauth_token.json<br/>Googleの認証トークン")]
    switchBot{{"SwitchBot API<br/>https://api.switch-bot.com"}}
    memoryStore[("memory<br/>長期メモリ")]
    wakeModeStore[("wakemode<br/>呼びかけモード")]
    volumeStore[("【再生音量】<br/>playbackvolume<br/>全端末の再生音量（メモリのみ）")]
    automationStore[("automations<br/>予約ジョブ")]
    locationStore[("【現在地】<br/>userlocation<br/>ユーザーの居場所")]
    browser{{"ブラウザ<br/>web/"}}

    rt -->|"EventToolRequest<br/>ツール実行依頼"| tool
    tool -->|"EventConversationCommitRequest<br/>ツール結果の会話反映依頼"| rt
    tool -->|"ツール呼び出し"| toolset
    scheduler -->|"ジョブのスクリプトを実行"| runner
    runner -->|"スクリプトからのツール呼び出し"| toolset

    webSearchTool <==>|"HTTPS<br/>Web検索"| openaiResponses
    webSearchTool -->|"Deliver<br/>後から届く検索結果"| gateway
    claudeCodeTool <==>|"HTTP<br/>依頼・承認・取消・参照"| agentBridge
    agentBridge -->|"HTTP<br/>Claude Codeの通知"| claudeCodeTool
    claudeCodeTool -->|"Deliver<br/>Claude Codeの通知"| gateway
    calendarTool <==>|"HTTPS<br/>予定の参照・作成・更新"| googleCalendar
    calendarTool <-.->|"読み出し・更新後の保存"| googleTokenStore
    calendarTool <==>|"HTTPS<br/>トークンの更新"| googleOAuth
    switchBotTool <==>|"HTTPS<br/>シーン実行"| switchBot
    memoryTool <-.->|"一覧・追加・更新・削除"| memoryStore
    wakeModeTool <-.->|"参照・切替"| wakeModeStore
    volumeTool <-.->|"切替"| volumeStore
    volumeStore <==>|"WebSocket<br/>/ws/playback-volume<br/>接続時・変更時の配信、画面からの変更"| browser
    automationTool <-.->|"登録・一覧・更新・削除"| automationStore
    locationTool -.->|"最後に受信した場所・更新日時の参照"| locationStore
    whiteboardTool ==>|"WebSocket<br/>会話履歴経由<br/>ホワイトボードの内容"| browser
```
