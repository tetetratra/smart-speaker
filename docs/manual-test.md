# 手動動作確認手順書（最小）

## 1. 目的/範囲
- 音声入力 → 応答テキスト → 音声再生が成功することを確認する
- ツール結果（timer）が返ることを確認する
- 画面/サーバーログでエラーが出ないことを確認する

## 2. 前提/制約
- macOS + Chrome 固定、ローカル環境のみ
- 環境変数・APIキーは事前設定済み
- Vosk のモデルと libvosk は Docker 内に含まれている
- 外部サービスのログインは人手対応可

## 3. 起動手順
1. サーバーを起動する
   - `docker build -t smart-speaker-server .`
   - `docker run --rm -p 8081:8081 -p 50000-50100:50000-50100/udp -e OPENAI_API_KEY=... -e ELEVENLABS_API_KEY=... -e ELEVENLABS_VOICE_ID=... -e ELEVENLABS_MODEL_ID=... -e RTC_ICE_HOST_IPS=... -e RTC_ICE_PORT_MIN=50000 -e RTC_ICE_PORT_MAX=50100 -e SWITCHBOT_TOKEN=... -e SWITCHBOT_SECRET=... -e SWITCHBOT_DEVICE_MAP=... smart-speaker-server`
   - Docker Compose 利用時:
     - `docker compose up --build`
2. フロントを起動する
   - `npm run dev`
3. Chrome で `http://localhost:5173/` を開く
4. Google OAuth の案内が出た場合は、画面の指示に従って認証を完了する（人手対応）

## 4. 操作フロー（音声入力での確認）
1. 画面の「接続」を押す
   - 「接続しました。話しかけてください。」が表示されること
   - 「音声接続」が「接続中」→「接続済み」になること
2. マイクに向かって発話する
   - 例: 「10秒後にタイマーをセットして」
3. 画面で以下を確認する
   - 応答テキストが表示される
   - `function call` に `name: schedule_timer` が表示される
   - `function result` が表示される
4. 約10秒後に system メッセージで「タイマー: ...」が表示される
5. 応答音声が再生される

## 5. 操作フロー（テキスト入力での代替確認）
音声入力が使えない場合の代替。音声入力の確認は人手で実施する。
1. 画面の「接続」を押す
2. テキスト入力欄に「10秒後にタイマーをセットして」を入力し「送信」を押す
3. 画面で以下を確認する
   - 応答テキストが表示される
   - `function call` に `name: schedule_timer` が表示される
   - `function result` が表示される
4. 約10秒後に system メッセージで「タイマー: ...」が表示される
5. 応答音声が再生される

## 6. 画面確認ポイント
- 「音声エラー」が表示されない
- user / assistant のメッセージが表示される
- `function call` / `function result` が表示される

## 7. サーバーログ確認ポイント
- `error` / `fatal` が出ていない
- Assistant の応答テキストが標準出力に出る

## 8. 成功判定基準
- 音声入力 → 応答テキスト → 音声再生の連続フローが成功する
- ツール結果（timer）が返る
- 画面/サーバーログでエラーが出ない

## 9. 失敗時の切り分け（最小）
- 接続できない: サーバー起動と `WS_ADDR`（デフォルト `:8081`）を確認
- 音声認識が動かない: Chrome のマイク権限と Docker コンテナの起動を確認
- 音声再生がない: ElevenLabs の API キー/VoiceID を確認
- `function result` が出ない: 発話に「タイマー」「時間指定」を含める

## 10. 例外対応
- Google OAuth の画面が出た場合は、人手で認証を完了してから再試行する
