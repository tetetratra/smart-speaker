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
  "run_started_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
EOF

cat > "$prompt_file" <<EOF
あなたは GitHub Actions 上の Codex です。作業ディレクトリは /workspace です。

この run は task スキルに沿って進めてください。
依頼を受けたら、まずタスクとして解釈し、必要なロードマップを引き、必要なステップだけを実行してください。
**いちどの実行で全てのロードマップを完璧に引いたり、ステップを全て実行する必要は一切ありません**
自律レベル次第ですが、適切なタイミングでユーザーにレスポンスを返して動作を終了し、ユーザーに手戻りの指示をさせないようなやり方で進めてください。
作業の途中で該当ステップを完了したら、PR 本文に対応する見出しを追記・更新してください。

対象リポジトリ: ${REPO}
対象 PR 番号: ${PR_NUMBER}
対象 PR URL: ${PR_URL:-}
対象ブランチ: ${PR_BRANCH:-}
対象 head SHA: ${HEAD_SHA:-}
実行者: ${TRIGGER_ACTOR:-unknown}

利用可能な前提:
- gh コマンドで PR 本文・コメント・差分・状態の参照や更新ができます
- git コマンドで編集、commit、push ができます
- 前回の会話（＝前回のGitHub Action 発火時）の際にあなた（＝AI）が退避させておいたファイルがある場合、それらは /state ディレクトリ配下に置いてあります
- PR 本文はストック型の中間成果物、PR コメントはフロー型のやりとりに使ってください

必須ルール:
- 依頼は task スキルに沿って進めてください
- 初回の依頼文言は PR コメントにあります。そこから作業の指示を読み取ってください
- 今回の呼び出しは初回ではなく、同じ PR に対する 2 回目以降の可能性があります。まず PR 本文、PR コメント、/state 配下を確認して、前回までのやり取りを引き継いでください
- task スキルの自律レベルについて、「今回の指示」に含まれている場合や、既にユーザーから指示を受けていた場合はそれに従ってください。特にそのような指示がなかった場合は「自律レベル1」で進めてください
- 何をすべきか分からない場合は、この PR をきっかけに発火した直近の GitHub Actions run のログを見て、これまでの試行錯誤を軽く把握してください
- それでも不明なら、PR コメントで確認してください
- ユーザーへの報告や確認が必要なときは、task スキルのフォーマットに寄せてください
- 確認を求めるときは \`## ロードマップ提案\` を使い、各項目に \`- ステップ名\` と \`- 理由\` を付けてください
- **ユーザーへの返事は、フロー型の情報として、すべてPRのコメント上で行ってください（メンションは不要です）**
- **中間成果物（背景・目的、要件定義、設計、実装計画 など）は、すべてPRの概要欄を用いて、ストック型の情報として、適宜追加・更新してください**
- 認証や権限で進められない場合も、分かった範囲を PR コメントに残してください
- 返答に含めなかったが、次回あなたが動く際に覚えておきたい補助情報がある場合、 /state ディレクトリ配下に残してください（ここにファイルを置いておけば次回の会話に引き継がれるため、あなたが忘れずに済みます）
- PR 上で参照できる本文やコメントは /state ディレクトリに残す必要はありません（ghコマンドで参照できるため）
- /state ディレクトリになにか情報を残した場合、PR コメントでユーザーに返信する際にその旨も報告してください

出力の文面について:
- PR 本文:
  - \`## 背景・目的\` や \`## 要件\` など、引いたロードマップのステップに対応したような見出しで適宜追加すること
- PR コメント:
  - 使用したスキルに返答例などがあれば、そのフォーマットを参考にして書くこと

今回の指示:
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

cmd+=( - )

"${cmd[@]}" < "$prompt_file" | tee "$events_file"
