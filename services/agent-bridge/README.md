# agent-bridge

手元のMacで常駐し、smart-speakerからの依頼でClaude Codeに作業させるブリッジです。
仕組みは `docs/tools/claude-code.md` を参照してください。

- Node.js 22.18以降が必要です。TypeScriptのファイルを、Nodeの型除去でそのまま実行します。ビルドは不要です。
- 依存は `@anthropic-ai/claude-agent-sdk` だけです。HTTPサーバーはNode標準の `node:http` で書いています。
- Claude CodeのCLIで、サブスクリプションにログイン済みである必要があります（`claude` を一度起動してログインします）。

## 設定（環境変数）
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

`ANTHROPIC_` と `CLAUDE` で始まる環境変数は、Claude Codeへ引き継ぎません。Claude Codeのデスクトップアプリから起動すると、これらの変数のせいでログインが使えなくなるためです。

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
1. `launchd/com.tetetratra.smart-speaker.agent-bridge.plist` を `~/Library/LaunchAgents/` にコピーし、パスと環境変数を書き換えます。
2. トークンはplistに書かず、`chmod 600` にしたファイルに置いて `AGENT_BRIDGE_TOKEN_FILE` で指定します。
3. `PATH` には `node` と `git` の場所を含めます。`git` は、セッションの一覧でworktreeを判定するためにSDKが使います。
4. 読み込みます。
   ```sh
   launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.tetetratra.smart-speaker.agent-bridge.plist
   ```

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
