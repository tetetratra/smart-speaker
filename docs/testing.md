# 動作確認メモ

## 1. 目的
- この資料は、`smart-speaker` の手動確認に必要な前提条件、起動パターン、確認手順、確認観点を最小限で整理するためのものです。
- 詳細な実装仕様や外部サービス設定の全量は扱いません。不明な点は不明として記載します。

## 2. 前提条件

### 共通
- 作業ディレクトリ: `/Users/kondo.daichi/p/smart-speaker`
- 参照元の `README.md` では以下が前提です。
  - Go `1.25` 以降
  - Node `20` 以降
  - `OPENAI_API_KEY`
  - `ELEVENLABS_API_KEY`
  - `ELEVENLABS_VOICE_ID`
- 任意の環境変数として `README.md` に記載があるもの:
  - `ELEVENLABS_MODEL_ID`
  - `RTC_ICE_HOST_IPS`
  - `WEB_DIST_DIR`
  - `WS_ADDR`
  - Google Calendar 利用時の OAuth 関連設定

### 音声入力・Google Speech 関連
- 旧資料では、音声入力確認に Google Speech 系の認証設定が必要とされています。
  - `GOOGLE_CLOUD_PROJECT`
  - `GOOGLE_APPLICATION_CREDENTIALS_JSON` または Application Default Credentials
- ただし、これらは現行 `README.md` には記載がありません。現行実装での必須条件はこの資料の参照範囲だけでは不明です。

### Google Calendar OAuth
- Google Calendar を確認する場合は、`README.md` の記載どおり以下を設定します。
  - `GOOGLE_CLIENT_ID`
  - `GOOGLE_CLIENT_SECRET`
  - `GOOGLE_REDIRECT_URL`（未設定時は `http://localhost:8081/oauth/google/callback`）

### ブラウザ・操作条件
- 旧資料では `macOS + Chrome` 前提です。
- マイク権限の許可、Google OAuth 同意画面の操作は手動対応を前提とします。

## 3. 起動パターン

### パターンA: Go サーバーのみ起動
- 実行コマンド:

```sh
go run ./cmd/smart-speaker
```

- `README.md` では、デフォルトで `WS_ADDR=:8081`、`/ws/chat` を開く構成です。
- この起動だけでは、フロント画面をどの URL で確認するかは状況依存です。
  - `web/dist` が存在する場合は `http://localhost:8081/` が候補です。
  - `web/dist` が未生成の場合の画面確認方法は、`README.md` だけでは十分に確定できません。

### パターンB: フロント開発用に Docker で `web` を起動
- 実行コマンド:

```sh
docker compose up web
```

- `README.md` では `npm install` と `npm run dev` が実行され、`http://localhost:5173/` で確認します。
- この記載はフロント単体の開発起動を示しています。Go サーバーを別途どう起動するかは、この記載単独では不明です。

### パターンC: 開発用 Docker Compose
- 旧資料では以下の起動パターンが使われています。

```sh
docker compose up --build
```

- 旧資料では、Go サーバーと Web UI をまとめて起動し、`http://localhost:5173/` を開く想定です。
- ただし、この起動パターンの現行妥当性は `README.md` からは断定できません。

### パターンD: 本番イメージ相当の起動
- 実行コマンド:

```sh
docker compose -f docker-compose.yml up --build
```

- `README.md` では、`npm run build` で生成された `web/dist` を Go サーバーが `/` で配信するとあります。
- 確認 URL は `http://localhost:8081/` です。

## 4. 手動確認手順

### 手順1: 起動確認
1. 利用する起動パターンを1つ選んで起動する。
2. サーバーが即時終了しないことを確認する。
3. 画面確認を行う場合は、起動パターンに応じた URL を開く。

確認ポイント:
- `OPENAI_API_KEY` 未設定など、必須環境変数不足で起動失敗していない。
- ElevenLabs の必須設定不足で起動失敗していない。

### 手順2: Web UI 接続確認
1. 画面を開く。
2. 接続操作を行う。
3. 接続状態がオンライン相当へ遷移することを確認する。

確認ポイント:
- WebSocket 接続が成立する。
- WebRTC を使う画面の場合、接続状態が接続済み相当へ遷移する。
- マイク権限ダイアログが出た場合、許可後に処理継続できる。

### 手順3: 音声入力の確認
1. マイク入力を有効化した状態で短い発話を行う。
2. 文字起こし結果が UI またはログで確認できるかを見る。
3. agent 応答テキストと音声再生を確認する。

確認ポイント:
- 音声入力がサーバーへ送信される。
- 文字起こし結果が user 発話として扱われる。
- agent 応答が返る。
- 応答音声が再生される。

補足:
- Google Speech 系の現行必須設定は参照資料だけでは確定できないため、音声入力が失敗した場合は認証設定差分の確認が必要です。

### 手順3.5: 応答 pipeline の簡易確認
実マイクや Google Speech-to-Text を使わず、テスト用の user 発話を pipeline に直接投入して、LLM 応答と TTS 音声生成まで確認できます。

```sh
go run ./cmd/local-verify-response
```

確認ポイント:
- `USER_TEXT=...` が出る。
- `ASSISTANT_TEXT=...` が出る。
- `AUDIO_BYTES_BASE64=...` が出る。
- graph log で `llm -> generationfilter -> tts -> scheduler -> router -> conversationcommitter` の流れが確認できる。

任意の入力文で確認したい場合は `LOCAL_VERIFY_TEXT` を指定します。

```sh
LOCAL_VERIFY_TEXT="こんにちは。短く自己紹介してください。" go run ./cmd/local-verify-response
```

この確認は OpenAI API と ElevenLabs API を実際に呼びます。
そのため、`OPENAI_API_KEY`、`ELEVENLABS_API_KEY`、`ELEVENLABS_VOICE_ID` が必要です。

### 手順4: Google Calendar OAuth の確認
1. Google Calendar を利用する場合は `http://localhost:8081/oauth/google/start` を開く、または画面上の Google 認証導線を使う。
2. 認証完了画面が表示されることを確認する。
3. `http://localhost:8081/oauth/google/status` を開き、認証状態を確認する。

確認ポイント:
- 認証完了後にトークンファイルが保存される。
- `authenticated: true` を確認できる。

## 5. 確認観点

### 接続
- サーバーが起動し続ける。
- WebSocket 接続が成立する。
- WebRTC 利用時は接続状態が安定する。

### 入力
- 音声入力が受け付けられる。

### 応答
- agent 応答テキストが返る。
- ElevenLabs による音声再生が行われる。

### ツール実行
- `function call` と `function result` が UI またはログで追える。
- tool 実行結果が後続メッセージとして返る。

### OAuth
- Google Calendar OAuth を開始できる。
- 認証完了後の状態を確認できる。

### ログ・障害
- `error`、`fatal`、`panic` が継続して出ない。
- 接続、文字起こし、TTS、ツール実行の失敗箇所をログから切り分けられる。

## 6. 将来の自動テスト観点
- 起動確認:
  - 必須環境変数不足時に明確なエラーで終了することを自動化しやすいです。
- HTTP エンドポイント確認:
  - `/oauth/google/start` と `/oauth/google/status` の疎通確認は自動化候補です。
- UI 接続確認:
  - Playwright などで、画面表示と接続状態遷移の確認を段階的に自動化できる余地があります。
- 音声経路の完全自動化:
  - 外部サービス依存が大きく、この資料の参照範囲だけでは現実的な自動化方針は不明です。

## 7. 参照元
- `README.md`
- 旧資料: `git show HEAD^:docs/0.手動動作確認手順書.md`
