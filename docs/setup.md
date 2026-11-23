# セットアップとローカル起動

> [!NOTE]
> このファイルは、手元で動かせるようにするための手順（必要なもの・設定ファイルの置き方・起動・テスト・外部サービス側でやる準備）だけを書きます。
> 設定項目の説明や既定値は [config.sample.yaml](../config.sample.yaml)、環境変数の一覧は [.env.sample](../.env.sample)、本番の運用は [deploy.md](deploy.md) に書き、アプリの動きの説明はここに書きません。

## 必要なもの

- Docker（Docker Compose）
- Docker を使わずに動かす・テストする場合は、次のものも必要です
  - Go 1.25 以降
  - Node.js 22.6 以降（`npm test` が `--experimental-strip-types` を使うため）
  - libopus・libopusfile・pkg-config（音声の変換で cgo から使います。macOS なら `brew install opus opusfile pkgconf`）
- OpenAI の API キー

## 設定

設定は、シークレットを環境変数で、それ以外を `config.yaml` で渡します。

```sh
cp config.sample.yaml config.yaml
cp .env.sample .env
```

`config.yaml` には変えたい項目だけを書きます。全項目と既定値は [config.sample.yaml](../config.sample.yaml) を参照してください。

- リポジトリ直下の `config.yaml` を読みます。環境変数 `SMART_SPEAKER_CONFIG` で別の場所も指定できます。
- ファイルが無い場合や項目が無い場合は、既定値を使います。
- `config.yaml` は Git の管理外です。

シークレットは、リポジトリ直下の `.env`（Git 管理外のファイル）に `名前=値` の形で書きます。項目は [.env.sample](../.env.sample) を参照してください。
必須は `OPENAI_API_KEY` だけで、ほかは使う機能に応じて設定します。

- `docker compose` が `.env` を読み、書いた変数をすべてサーバーのコンテナに渡します。
- 新しい変数（`config.yaml` の `${...}` で参照する MCP サーバーのシークレットなど）を足すときも、`.env` に書くだけで済みます。
- Docker を使わずに `go run` で動かす場合や、Mac で agent-bridge を動かす場合は、シェルの環境変数として値が要ります。リポジトリ直下の `.envrc`（direnv で読む、Git 管理外のファイル）に `dotenv` と書くと、`.env` を読み込めます。

## 起動

```sh
docker compose up --build
```

`http://localhost:5173` を開きます（マイクの許可が必要です）。
`docker compose up` は `docker-compose.override.yml` も自動で読むため、サーバーは `go run` で、画面は Vite の開発サーバーで動きます。VOICEVOX とローカル STT も一緒に起動します。

スマートフォンなど別の端末から開く場合は、`config.yaml` の `rtc.ice_advertise_ips` に開発 PC の LAN IP を書きます。

## テスト

```sh
npm ci
npm test
npm run build
go test -race ./...
python3 -m unittest discover -s services/local-stt
```

## 外部 API を使わないローカル確認

偽の Realtime API と VOICEVOX 互換のサービスにつないで、アプリを起動します。本物の API キーや家電の操作は使いません。

```sh
npm ci
npm run build
go run ./cmd/local-verify-realtime
```

`http://127.0.0.1:18081` を開いて接続します。
別のターミナルから発話を送れます。音が鳴っている間に 2 つ目を送ると、割り込みを再現できます。

```sh
curl -X POST http://127.0.0.1:18082/debug/utterance -d '{"text":"最初の会話"}'
curl -X POST http://127.0.0.1:18082/debug/utterance -d '{"text":"割り込み後の会話"}'
```

「ホワイトボード:」で始まる発話を送ると、モデルがツール（`set_whiteboard`）を呼ぶ応答を返します。ツールの経路を確かめるときに使います。

```sh
curl -X POST http://127.0.0.1:18082/debug/utterance -d '{"text":"ホワイトボード:今日の予定"}'
```

## 外部サービス連携の設定

### Google Calendar

1. Google Cloud で OAuth クライアントを作り、`GOOGLE_CLIENT_ID` と `GOOGLE_CLIENT_SECRET` を設定します。
2. コールバック URL を、OAuth クライアントの承認済みリダイレクト URI に登録します。既定は `http://localhost:8081/oauth/google/callback` で、変える場合は `config.yaml` の `plugins.googlecalendar.redirect_url` に書きます。
3. サーバーを起動し、`http://localhost:8081/oauth/google/start` を開いて認可します。本番サーバーで認可するときは、[デプロイ](deploy.md#外部サービスの認可と準備)を参照してください。

### MCP サーバー

`config.yaml` の `plugins.mcp.servers` に MCP サーバーを書くと、そのツールを会話から使えます（書き方は `config.sample.yaml`）。
設定はサーバーの再起動で反映します。

- リモート（`url`）: Streamable HTTP の MCP サーバーにつなぎます。固定のトークンは `headers` に `${環境変数}` で書きます。
- ローカル（`command`）: stdio の MCP サーバーを子プロセスとして起動します。本番で使う場合は、[デプロイ](deploy.md#外部サービスの認可と準備)を参照してください。

OAuth が要るリモートの MCP サーバーは、`oauth:` を書いて次の手順で認可します。サーバーは MCP 標準の保護リソースのメタデータ（`/.well-known/oauth-protected-resource`）を公開している必要があります。

1. `plugins.mcp.redirect_url` に、画面を開いている HTTPS の住所のコールバック URL（`https://<ドメイン>/oauth/mcp/callback`）を書きます。独自のドメインでも、`tailscale serve` の `https://<マシン名>.<tailnet名>.ts.net` でも構いません。
2. 動的クライアント登録に対応していないサーバー（Google など）は、提供元で OAuth クライアントを作って `oauth.client_id`・`oauth.client_secret` に書き、リダイレクト URI に 1 の URL を登録します。
3. 画面の「設定」を開き、未認可のサーバーの「認可する」を押します。提供元の画面で許可すると、設定画面へ戻ります。
4. サーバーを再起動すると、ツールが出ます。

### SwitchBot

1. SwitchBot アプリでトークンとシークレットを発行し、`SWITCHBOT_TOKEN` と `SWITCHBOT_SECRET` を設定します。
2. 温度・湿度・明るさを会話に伝える場合は、`config.yaml` の `plugins.switchbot.device_map` に `hub2` のデバイス ID を書きます。デバイス ID は `scripts/switchbot_deviceids.rb` で一覧できます。

シーンは起動時に SwitchBot から取得します。シーンを足したら、サーバーを再起動します。

### 読み上げの声

- VOICEVOX の声は `tts.voicevox.speaker_id` で選びます。
- ElevenLabs を使う場合は、`ELEVENLABS_API_KEY` と `tts.elevenlabs.voice_id` を設定します。

### Claude Code

Mac で [agent-bridge](../services/agent-bridge/README.md) を常駐させ、サーバーには `config.yaml` の `plugins.claudecode.url` と環境変数 `AGENT_BRIDGE_TOKEN` を設定します。

### 現在地 API

iPhone のショートカットなどから、今いる場所を文字列で送れます。
アプリ独自の認証はないため、自宅 LAN か Tailscale 経由で使います。

```sh
curl -X POST http://<サーバー>:8081/api/location \
  -H 'Content-Type: application/json' \
  -d '{"place":"home"}'
```

ショートカットでは「URL の内容を取得」で、方法を POST、要求本文を JSON にし、`place` に場所の名前を入れます。
