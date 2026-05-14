#!/usr/bin/env bash
set -euo pipefail

if [ -z "${STATE_ROOT:-}" ]; then
  echo "STATE_ROOT is required" >&2
  exit 1
fi

if [ -z "${PR_NUMBER:-}" ]; then
  echo "PR_NUMBER is required" >&2
  exit 1
fi

if [ -z "${REPO:-}" ]; then
  echo "REPO is required" >&2
  exit 1
fi

if [ -z "${INSTRUCTION_FILE:-}" ]; then
  echo "INSTRUCTION_FILE is required" >&2
  exit 1
fi

mkdir -p "$STATE_ROOT/run" "$STATE_ROOT/result"

prompt_file="$STATE_ROOT/run/prompt.txt"
events_file="$STATE_ROOT/run/events.jsonl"
final_file="$STATE_ROOT/result/final.md"
meta_file="$STATE_ROOT/run/meta.json"

cat > "$meta_file" <<EOF
{
  "pr_number": "${PR_NUMBER}",
  "pr_url": "${PR_URL:-}",
  "branch_name": "${PR_BRANCH:-}",
  "head_sha": "${HEAD_SHA:-}",
  "trigger_actor": "${TRIGGER_ACTOR:-}",
  "autonomy_level": "${AUTONOMY_LEVEL:-level1}",
  "run_started_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
EOF

cat > "$prompt_file" <<EOF
あなたは GitHub Actions 上の Codex です。作業ディレクトリは /workspace です。

この run は \`task\` スキルに沿って進めてください。依頼を受けたら、まずタスクとして解釈し、必要なロードマップを引き、必要なステップだけを実行してください。作業の途中で該当ステップを完了したら、PR 本文に対応する見出しを追記・更新してください。

対象リポジトリ: ${REPO}
対象 PR 番号: ${PR_NUMBER}
対象 PR URL: ${PR_URL:-}
対象ブランチ: ${PR_BRANCH:-}
対象 head SHA: ${HEAD_SHA:-}
実行者: ${TRIGGER_ACTOR:-unknown}
自律レベル: ${AUTONOMY_LEVEL:-level1}

利用可能な前提:
- gh コマンドで PR 本文・コメント・差分・状態を参照できます
- git コマンドで編集、commit、push ができます
- 継続状態は /state にあります
- /state には PR 上から再取得できる情報を複製せず、補助メモ・気づき・一時スクリプト・未コミット下書きだけを保存してください
- PR 本文はストック型の中間成果物、PR コメントはフロー型のやりとりに使ってください

必須ルール:
- 依頼は \`task\` スキルに沿って進めてください
- PR 本文は空から始まっても問題ありません。作業した内容に応じて、必要な見出しだけを追記・更新してください
- 自律レベルは入力値に従って \`task\` スキルの自律レベルとして扱ってください
- 最終的に gh コマンドで PR コメントを投稿し、見出しとして「要約」「結果」「次アクション」を含めてください
- 必要なら gh コマンドで PR 本文を更新し、ストック型の中間成果物を最新化してください
- 認証や権限で進められない場合も、分かった範囲を PR コメントに残してください
- 返答に含めなかった補助情報だけを /state/memory, /state/notes, /state/scratch, /state/result, /state/run に残してください
- PR 上で参照できる本文やコメント全文を /state にコピーしないでください

今回の追加指示:
EOF

cat "$INSTRUCTION_FILE" >> "$prompt_file"

skills_src="/workspace/.codex/skills"
skills_dest="${CODEX_HOME}/skills"
if [ -d "$skills_src" ]; then
  mkdir -p "$skills_dest"
  cp -R "$skills_src"/. "$skills_dest"/
fi

cmd=(
  codex exec
  --dangerously-bypass-approvals-and-sandbox
  --ignore-user-config
  --json
  --color never
  --output-last-message "$final_file"
  -C /workspace
)

if [ -n "${CODEX_MODEL:-}" ]; then
  cmd+=( -m "$CODEX_MODEL" )
fi

cmd+=( - )

"${cmd[@]}" < "$prompt_file" | tee "$events_file"
