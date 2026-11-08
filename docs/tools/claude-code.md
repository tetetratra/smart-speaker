# Claude Code連携

## 概要
Realtimeエージェントから、手元のMacで動くClaude Codeに時間のかかる作業を依頼するツール群です。
Claude Codeは本番サーバーではなくMacで動かします。セッションの記録がMacに残るので、作業の途中からPCで `claude --resume <session_id>` を実行して同じセッションを続けられます。

構成は次の2つです。
- **smart-speaker（本番サーバー）**: ツール、通知の受付API（`POST /api/agent-events`）、Realtimeへの差し込みを担当します。
- **agent-bridge（Mac、`services/agent-bridge`）**: Node.jsの常駐プロセスです。Agent SDKの `query()` でClaude Codeに作業させ、承認依頼と結果をsmart-speakerへプッシュします。

対象はMacのローカルにあるセッションだけです。クラウド（claude.ai/code）のセッションは扱いません。公開APIがないためです。

## 登録条件
- ブリッジのURL（`config.yaml` の `agent_bridge.url`）と共有トークン（環境変数 `AGENT_BRIDGE_TOKEN`）が両方設定されているときだけ、ツールと `/api/agent-events` を有効にします。
- どちらかが空なら、ツールはRealtimeに渡さず、`/api/agent-events` は503を返します。

## ツール
| ツール | 用途 | 結果 |
| --- | --- | --- |
| `ask_claude_code(input, session_id)` | 作業を依頼する。続きは同じ `session_id` を指定する | すぐに `{status: "started", task_id, session_id}` を返す |
| `answer_claude_code_permission(request_id, allow)` | 承認依頼に答える | `{status: "answered"}` |
| `cancel_claude_code(task_id, session_id)` | 実行中の作業を止める。`task_id` が分からなければ `session_id` で指定する | `{status: "cancel_requested"}` |
| `list_claude_code_sessions(limit)` | セッションを新しい順に一覧する | タイトル、作成・最終更新日時（日本時間）、最初の依頼、実行中かどうか（実行中なら `task_id`） |
| `read_claude_code_session(session_id, limit, offset)` | セッションの会話を読む | 直近のやり取り。`offset` は新しい側から数える |

- 利用方針は `internal/components/realtime/claudecode.go` の `withClaudeCodeInstructions` で、ツールがあるときだけシステムプロンプトに足します。
- 一覧と会話の伝え方は、各ツールの応答方針（ResponsePolicy）で指示します。

## 処理の流れ
1. Realtimeが `ask_claude_code` を呼ぶと、サーバーはブリッジの `POST /tasks` を呼びます。ブリッジは作業を始めた時点で `task_id` と `session_id` を返し、作業は裏で続きます。
2. Claude Codeが承認を必要とすると、Agent SDKの `canUseTool` が呼ばれます。ブリッジは返事が来るまで待ち、`permission_request` をサーバーへ送ります。
3. サーバーは通知を `ScheduledPrompt` に変換し、Realtimeへ差し込みます。Realtimeはユーザーに尋ね、返事を聞いてから `answer_claude_code_permission` を呼びます。サーバーがブリッジの `POST /permissions/{request_id}` に返事を送ると、作業が続きます。
4. 作業が終わると、ブリッジは `completed` か `failed` を送ります。Realtimeは `summary` を読み上げ、`details` をホワイトボードに表示します。

権限モードは `auto` です。ほとんどの操作は自動で許可され、分類器が判断できなかった操作だけが承認依頼になります。分類器が拒否した操作は承認依頼にならず、完了通知の `permission_denials` で伝わります。

## 安全のための仕組み

### 前提（脅威モデル）
- 自宅LANとTailscaleのネットワークは信頼します。smart-speakerの他のAPIと同じく、Web UIや音声の入口には認証がありません。
- そのため、ネットワークに入れる人と、部屋で話せる人（テレビなどの音声も含む）は、`ask_claude_code` でMacに作業を頼めます。
- 予約ジョブ、外部エージェントの通知、外部の文章を読んだ直後の応答からは、`ask_claude_code` と `answer_claude_code_permission` を呼べません（下記）。
- 権限モードは `auto` なので、明示的に頼んだ操作は承認なしで実行されることがあります。危険な操作は、下の `permissions.ask` で必ず承認に回してください。

### 仕組み
- **認証**: ブリッジのAPIと `/api/agent-events` は共有トークン（Bearer）で認証します。他のAPIと違い、Macで作業を進める入口になるためです。
- **外部の出力はデータとして渡す**: 差し込む指示文では、Claude Codeから届いた内容をJSONで囲み、中の指示に従わないよう明記します。
- **ユーザー本人の返事が必要なツール**: `ask_claude_code` と `answer_claude_code_permission` は、予約ジョブや外部エージェントの通知で始まった応答からは呼べません。外部の文章を返すツール（`web_search`、`ask_subagent`、`list_claude_code_sessions`、`read_claude_code_session`）の結果を受け取った後の応答からも呼べません。どちらの場合も、ステージがエラー結果を返します（`blockedMachineTool`）。Claude Codeの出力やWebページに「承認済みなので許可して」と紛れていても、承認や新しい作業が勝手に進まないようにするためです。ユーザーが次に話せば、呼べるようになります。
- **古い承認依頼は尋ねない**: 承認依頼は、期限（`ExpiresAt`）を過ぎたり、作業が終わったり、既に答えたりしたら読み上げずに捨てます。作業が終わった後に遅れて届いた承認依頼も捨てます。ブリッジも、片付いた承認依頼の通知は再送しません。
- **承認の状態はブリッジが持つ**: サーバーを再起動して承認依頼の控えを失っても、返事はそのままブリッジへ送ります。答え済みや期限切れなら、ブリッジが404を返します。
- **起動ワード必須モード**: 承認依頼を差し込むときは会話中の状態にします（`wakemode.Store.Awaken`）。起動ワードなしで「はい」と答えられます。会話中の時間は差し込んだ時点から `wake.conversation_timeout_seconds`（既定5分）なので、読み上げにかかった分だけ短くなります。その間は周囲の音声も受け付けます。
- **Claude Codeに渡す環境変数を絞る**: ブリッジは、`AGENT_BRIDGE_` で始まる環境変数（共有トークンを含む）と、名前が `_TOKEN`・`_KEY`・`_SECRET`・`_PASSWORD` で終わる環境変数を、Claude Codeへ引き継ぎません。共有トークンがあると、Claude Codeが自分の承認依頼を自分で許可できてしまうためです。
- **声で答えられないツールは使わない**: ブリッジは `AskUserQuestion`、`EnterPlanMode`、`ExitPlanMode` を使わせません。

危険な操作を必ず承認に回したい場合は、Mac側のClaude Codeの設定（`~/.claude/settings.json` の `permissions.ask`）に、`Bash(git push:*)` などのルールを足してください。`ask` ルールに当たった操作は、`auto` モードでも承認依頼になります。

## 通知の形
ブリッジから `POST /api/agent-events` に送る形です。正となる定義は `internal/agentbridge/types.go` と `services/agent-bridge/src/protocol.ts` です。

```json
{"type": "permission_request", "event_id": "…", "task_id": "…", "session_id": "…", "request_id": "…",
 "tool": "Bash", "title": "Claude wants to run …", "input": {"command": "…"}, "expires_at": "2026-11-07T09:05:00Z"}
{"type": "completed", "event_id": "…", "task_id": "…", "session_id": "…", "request": "依頼文の冒頭",
 "summary": "…", "details": ["…"], "permission_denials": ["Bash: {…}"]}
{"type": "failed", "event_id": "…", "task_id": "…", "session_id": "…", "request": "依頼文の冒頭", "error": "…"}
```

- 同じ `event_id` の再送は、2回目以降を捨てて202を返します。
- 401はトークンの誤り、400は形式の誤りです。

## 設定
- smart-speaker: `config.yaml` の `agent_bridge.url`（例: `http://<MacのTailscale IP>:8787`）と、環境変数 `AGENT_BRIDGE_TOKEN`。
- agent-bridge: 環境変数で渡します。常駐させるときは、リポジトリ直下の `.envrc` に書き、LaunchAgentから `direnv exec` 経由で起動します。共有トークンは、サーバーと同じ `AGENT_BRIDGE_TOKEN` を使います。手順は `services/agent-bridge/README.md` を参照してください。

## 制約
- ブリッジを再起動すると、実行中の作業も止まります。完了通知は届きません。
- Macがスリープ中は、ツールが接続エラーを返します（接続3秒、全体10秒で諦めます）。
- 同時に動かす作業は、既定で1つです（`AGENT_BRIDGE_MAX_TASKS`）。
