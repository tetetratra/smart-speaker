# agent-bridge

手元のMacで常駐し、smart-speakerからの依頼でClaude Codeに作業させるブリッジです。
仕組みは `docs/tools/claude-code.md` を参照してください。

- Node.js 22.18以降が必要です。TypeScriptのファイルを、Nodeの型除去でそのまま実行します。ビルドは不要です。
- 依存は `@anthropic-ai/claude-agent-sdk` だけです。HTTPサーバーはNode標準の `node:http` で書いています。
- Claude CodeのCLIで、サブスクリプションにログイン済みである必要があります（`claude` を一度起動してログインします）。

## 設定（環境変数）
設定は環境変数で渡します。常駐させるときは、リポジトリ直下の `.envrc`（direnvで読む、Git管理外のファイル）に書きます。
サーバーのデプロイも同じ `.envrc` を読むため、共有トークン `AGENT_BRIDGE_TOKEN` は1か所に書くだけで済みます。

| 変数 | 必須 | 説明 |
| --- | --- | --- |
| `AGENT_BRIDGE_LISTEN` | ○ | 待ち受けるアドレス。例: `100.x.y.z:8787`（MacのTailscale IP） |
| `AGENT_BRIDGE_TOKEN` または `AGENT_BRIDGE_TOKEN_FILE` | ○ | smart-speakerと共有するトークン（16文字以上）。ファイルから読む場合は後者 |
| `AGENT_BRIDGE_WORKDIR` | ○ | Claude Codeの作業ディレクトリ。一覧に出すのもこのディレクトリのセッションだけ |
| `AGENT_BRIDGE_EVENTS_URL` | ○ | 通知先。例: `http://<サーバーのTailscale IP>:8081/api/agent-events` |
| `AGENT_BRIDGE_CLAUDE_PATH` | | 使う `claude` の場所。未設定ならSDK同梱のClaude Code |
| `AGENT_BRIDGE_PERMISSION_TIMEOUT_SECONDS` | | 承認の返事を待つ秒数。既定300 |
| `AGENT_BRIDGE_TASK_TIMEOUT_SECONDS` | | 1つの作業の上限秒数。既定3600 |
| `AGENT_BRIDGE_MAX_TASKS` | | 同時に動かす作業の数。既定1 |

次の環境変数は、Claude Codeへ引き継ぎません。

- `ANTHROPIC_` と `CLAUDE` で始まるもの。Claude Codeのデスクトップアプリから起動すると、これらの変数のせいでログインが使えなくなるためです。
- `AGENT_BRIDGE_` で始まるもの。共有トークンがあると、Claude Codeが自分の承認依頼を自分で許可できてしまうためです。
- 名前が `_TOKEN`、`_KEY`、`_SECRET`、`_PASSWORD` で終わるもの。`.envrc` にあるサーバー用のシークレット（`OPENAI_API_KEY` など）を渡さないためです。

## 起動
```sh
cd services/agent-bridge
npm install
AGENT_BRIDGE_LISTEN=127.0.0.1:8787 AGENT_BRIDGE_TOKEN=... AGENT_BRIDGE_WORKDIR=$HOME/work \
  AGENT_BRIDGE_EVENTS_URL=http://127.0.0.1:8081/api/agent-events npm start
```

テストと型チェック:
```sh
npm test
npm run typecheck
```

## LaunchAgentで常駐させる
worktreeは消えることがあるので、以下はリポジトリの本来の場所（メインのチェックアウト）で行います。

1. リポジトリ直下の `.envrc` に、上の表の必須の変数を書き、`direnv allow` で許可します。
   ```sh
   export AGENT_BRIDGE_TOKEN=<openssl rand -hex 32 などで作った値>
   export AGENT_BRIDGE_LISTEN=<MacのTailscale IP>:8787
   export AGENT_BRIDGE_WORKDIR=$HOME/work
   export AGENT_BRIDGE_EVENTS_URL=http://<サーバーのTailscale IP>:8081/api/agent-events
   ```
2. 依存を入れます。
   ```sh
   (cd services/agent-bridge && npm install)
   ```
3. LaunchAgentの設定ファイル（plist）を書き出します。リポジトリ直下で実行すると、`node` と `direnv` の場所、リポジトリの場所が埋まります。
   ブリッジは `direnv exec` 経由で起動し、`.envrc` の環境変数を受け取ります。
   `PATH` には `git` の場所を含めます。`git` は、セッションの一覧でworktreeを判定するためにSDKが使います。
   ```sh
   NODE="$(node -p process.execPath)"
   DIRENV="$(command -v direnv)"
   cat > ~/Library/LaunchAgents/com.tetetratra.smart-speaker.agent-bridge.plist <<EOF
   <?xml version="1.0" encoding="UTF-8"?>
   <!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
   <plist version="1.0">
   <dict>
     <key>Label</key>
     <string>com.tetetratra.smart-speaker.agent-bridge</string>
     <key>ProgramArguments</key>
     <array>
       <string>$DIRENV</string>
       <string>exec</string>
       <string>$PWD</string>
       <string>$NODE</string>
       <string>$PWD/services/agent-bridge/src/main.ts</string>
     </array>
     <key>WorkingDirectory</key>
     <string>$PWD/services/agent-bridge</string>
     <key>EnvironmentVariables</key>
     <dict>
       <key>PATH</key>
       <string>$(dirname "$NODE"):$(dirname "$DIRENV"):/usr/bin:/bin</string>
       <key>LANG</key>
       <string>ja_JP.UTF-8</string>
     </dict>
     <key>RunAtLoad</key>
     <true/>
     <key>KeepAlive</key>
     <true/>
     <key>ThrottleInterval</key>
     <integer>10</integer>
     <key>StandardOutPath</key>
     <string>$HOME/Library/Logs/smart-speaker-agent-bridge.log</string>
     <key>StandardErrorPath</key>
     <string>$HOME/Library/Logs/smart-speaker-agent-bridge.log</string>
   </dict>
   </plist>
   EOF
   plutil -lint ~/Library/LaunchAgents/com.tetetratra.smart-speaker.agent-bridge.plist
   ```
4. 読み込みます。
   ```sh
   launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.tetetratra.smart-speaker.agent-bridge.plist
   ```

`.envrc` を変えたときは、再起動して読み直させます。
```sh
launchctl kickstart -k gui/$(id -u)/com.tetetratra.smart-speaker.agent-bridge
```

plistを変えたときや、`node` を別のバージョンに上げたとき（plistには手順3の時点の `node` の場所が入ります）は、`launchctl bootout gui/$(id -u)/com.tetetratra.smart-speaker.agent-bridge` で外してから、手順3と4をやり直します。

Tailscaleより先に起動するとアドレスを確保できずに終了しますが、`KeepAlive` で再起動します。
Tailscaleのアクセス制御で、サーバーからMacの待ち受けポートへの通信だけを許可しておくと安全です。

## API
すべて `Authorization: Bearer <トークン>` が必要です（`/healthz` を除く）。

| メソッドとパス | 用途 |
| --- | --- |
| `POST /tasks {input, session_id}` | 作業を始める。`202 {task_id, session_id}`。409は同じセッションが作業中、429は同時実行数の上限、404はセッションがない |
| `POST /permissions/{request_id} {allow, message}` | 承認依頼に答える。`204`。404は保留中の依頼がない |
| `POST /tasks/{task_id}/cancel` | 作業を止める。`204` |
| `POST /sessions/{session_id}/cancel` | セッションで実行中の作業を止める。`204`。404は実行中の作業がない |
| `GET /tasks/{task_id}` | 作業の状態（動作確認用） |
| `GET /sessions?limit=20` | 作業ディレクトリのセッション一覧（新しい順）。実行中なら `task_id` も入る |
| `GET /sessions/{session_id}/messages?limit=10&offset=0` | 会話の内容。`offset` は新しい側から数える |
| `GET /healthz` | 死活確認 |
