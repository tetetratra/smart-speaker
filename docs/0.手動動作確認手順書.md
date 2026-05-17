# 手動動作確認手順書（最小）

## 1. 目的/範囲
- Web UI から WebSocket / WebRTC 接続が確立し、マイク音声がサーバーへ送信されることを確認する
- 音声入力 → Google 文字起こし → assistant 応答テキスト → ElevenLabs 音声再生が成功することを確認する
- テキスト入力でも assistant 応答とツール実行結果を確認できることを確認する
- Google Calendar OAuth の開始・完了・認証状態を確認する
- ツール結果（timer）が返り、指定秒数後のタイマー通知が表示されることを確認する
- 画面/サーバーログ/会話ログで致命的なエラーが出ないことを確認する

## 2. 前提/制約
- macOS + Chrome 固定、ローカル環境のみ
- `cd /Users/kondo.daichi/p/smart-speaker` 済みのシェルで実施する
- API キーや認証情報は事前設定済み
  - サーバー起動必須: `OPENAI_API_KEY`, `ELEVENLABS_API_KEY`, `ELEVENLABS_VOICE_ID`
  - 音声入力確認必須: `GOOGLE_CLOUD_PROJECT`
  - 音声入力の認証情報: `GOOGLE_APPLICATION_CREDENTIALS_JSON`、または Application Default Credentials
  - Google Calendar OAuth 確認必須: `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`
  - Docker で WebRTC を使う場合: 必要に応じて `RTC_ICE_HOST_IPS`
- Google Speech の認識言語は未指定なら `ja-JP`、認識器は未指定なら `_`
- Google Calendar OAuth の redirect URL は未指定なら `http://localhost:8081/oauth/google/callback`
- 外部サービスのログイン、Chrome のマイク権限許可、Google OAuth 同意画面は人手対応可

## 3. 起動手順
1. リポジトリへ移動する
   - `cd /Users/kondo.daichi/p/smart-speaker`
2. 開発確認として Docker Compose で起動する場合
   - `docker compose up --build`
   - サーバーは `go run ./cmd/smart-speaker`、Web UI は `npm install && npm run dev -- --host --config web/vite.config.ts` で起動される
   - Chrome で `http://localhost:5173/` を開く
3. ローカルプロセスで分けて起動する場合
   - サーバー: `go run ./cmd/smart-speaker`
   - フロント: `npm install`（初回のみ）後に `npm run dev`
   - Chrome で `http://localhost:5173/` を開く
4. 本番イメージ相当で確認する場合
   - `docker compose -f docker-compose.yml up --build`
   - Chrome で `http://localhost:8081/` を開く
   - この場合は Go サーバーがビルド済み Web UI を `/` で配信する
5. Google Calendar OAuth を確認する場合
   - 管理画面の「Google認証」を押す、または `http://localhost:8081/oauth/google/start` を開く
   - 認証完了画面に「Google認証が完了しました」が表示されること
   - `http://localhost:8081/oauth/google/status` を開き、`"authenticated":true` が返ること

## 4. 操作フロー（音声入力での確認）
1. `http://localhost:5173/` または `http://localhost:8081/` を開く
2. 通常画面で「接続」トグルを押す
   - 接続表示が「接続中」から「オンライン」になること
   - 「マイク」が「待機中」または「検知中」になること
   - 「認識」が「待機中」「認識中」「最終結果待ち」「完了」のいずれかへ遷移すること
   - Chrome のマイク権限ダイアログが出た場合は許可する
3. 「管理画面」を押し、詳細表示へ移動する
   - `WebRTC接続` が「接続済み」になること
   - `マイクストリーム送信` が「待機中」または「送信中」になること
   - `サーバー発話検知` が発話時に「検知中」へ遷移すること
   - `Google文字起こし` が発話後に「認識中」→「最終結果待ち」→「完了」へ遷移すること
4. マイクに向かって発話する
   - 例: 「10秒後にタイマーをセットして」
5. 管理画面で以下を確認する
   - `User (server-stt)` のメッセージに文字起こし結果が表示される
   - `Assistant` の応答テキストが表示される
   - `function call` に `name: schedule_timer` が表示される
   - `function result` に `name: schedule_timer` と `scheduled_for` / `seconds` が表示される
6. 約10秒後に `Assistant (timer)` メッセージで「タイマーが発火しました: ...」が表示される
7. 応答音声が再生される

## 5. 操作フロー（テキスト入力での代替確認）
音声入力が使えない場合の代替。音声入力の確認は人手で別途実施する。
1. 管理画面を開く
   - 通常画面の「管理画面」を押す、または `http://localhost:5173/?ui=admin` を開く
2. 「接続」を押す
   - メッセージ一覧に「接続しました。話しかけてください。」が表示されること
   - `WebRTC接続` が「接続中」から「接続済み」になること
3. テキスト入力欄に「10秒後にタイマーをセットして」を入力し「送信」を押す
4. 画面で以下を確認する
   - `User` のメッセージに送信テキストが表示される
   - `Assistant` の応答テキストが表示される
   - `function call` に `name: schedule_timer` が表示される
   - `function result` に `name: schedule_timer` と `scheduled_for` / `seconds` が表示される
5. 約10秒後に `Assistant (timer)` メッセージで「タイマーが発火しました: ...」が表示される
6. 応答音声が再生される

## 6. 画面確認ポイント
- 通常画面に最新の user / assistant 発話が表示される
- 通常画面の「接続」が「オンライン」になり、音量/しきい値が表示される
- 管理画面で `WebRTC接続` / `マイクストリーム送信` / `サーバー発話検知` / `Google文字起こし` の状態が確認できる
- 管理画面に `function call` / `function result` が表示される
- 管理画面に `音声エラー` / `文字起こしエラー` が表示されない
- Google OAuth 完了後、`/oauth/google/status` で `authenticated` が `true` になる

## 7. サーバーログ確認ポイント
- 起動直後に `OPENAI_API_KEY is not set` や `failed to init elevenlabs stage` で停止していない
- Google OAuth トークン未作成時は `google oauth token not found. open /oauth/google/start to authenticate` が出るが、OAuth 確認前なら許容する
- ローカル開発で Vite を使う場合、`web ui: dist dir not found` は Go サーバー配信を使わないため許容する
- WebRTC 確立時に `rtc: connection state=connected` が出る
- 発話時に `rtc: speech start` / `rtc: speech end` が出る
- assistant 応答や timer 通知が graph ログの `EventRealtimeOutput{text=...}` として出る
- ツール実行時に `EventToolRequest{name=schedule_timer,...}` と `EventToolResponse{output=...}` が出る
- ElevenLabs 音声生成時に `elevenlabs: tts duration=... bytes=...` が出る
- `error` / `fatal` / `panic` が継続的に出ていない
- 会話ログ `data/conversation.jsonl` に user / assistant の JSONL レコードが追記される

## 8. 成功判定基準
- WebSocket と WebRTC が接続済みになる
- 音声入力 → Google 文字起こし → 応答テキスト → 音声再生の連続フローが成功する
- テキスト入力でも応答テキストと音声再生が成功する
- `schedule_timer` の `function call` / `function result` が管理画面に表示される
- 指定秒数後に `Assistant (timer)` として「タイマーが発火しました: ...」が表示される
- Google OAuth が完了し、`/oauth/google/status` で `authenticated: true` を確認できる
- 画面/サーバーログ/会話ログで致命的なエラーが出ない

## 9. 失敗時の切り分け（最小）
- 画面を開けない: 起動方法に応じて `http://localhost:5173/` または `http://localhost:8081/` を開いているか確認
- WebSocket 接続できない: サーバー起動、`WS_ADDR`（デフォルト `:8081`）、`/ws/chat`、Docker の `8081:8081` 公開を確認
- WebRTC が接続済みにならない: Chrome のマイク権限、Docker の UDP `50000-50100` 公開、必要に応じて `RTC_ICE_HOST_IPS` を確認
- 音声認識が動かない: `GOOGLE_CLOUD_PROJECT` と `GOOGLE_APPLICATION_CREDENTIALS_JSON`、Chrome のマイク権限、`Google文字起こし` のエラー表示を確認
- 音声再生がない: `ELEVENLABS_API_KEY` / `ELEVENLABS_VOICE_ID` / `ELEVENLABS_MODEL_ID`、ブラウザの音量、通常画面の再生音量スライダーを確認
- Google OAuth が失敗する: `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` / `GOOGLE_REDIRECT_URL` と Google 側の承認済みリダイレクト URI を確認
- `function result` が出ない: 入力に「10秒後」「タイマー」のような時間指定とタイマー要求を含める
- タイマー通知が出ない: `function result` の `seconds` が期待値か確認し、約10秒後に `Assistant (timer)` が出るか確認

## 10. 例外対応
- Google OAuth の画面が出た場合は、人手で認証を完了してから再試行する
- OAuth トークンを作り直す場合は、`GOOGLE_OAUTH_TOKEN_PATH` のファイル（未指定時は `data/google_oauth_token.json`）を削除してから再認証する
- 音声入力が不安定な場合は、テキスト入力でツール実行と TTS を先に確認し、その後 Chrome のマイク権限と Google Speech 設定を切り分ける
