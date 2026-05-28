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

if [ -z "${CURSOR_API_KEY:-}" ]; then
  echo "CURSOR_API_KEY is required" >&2
  exit 1
fi

mkdir -p "$STATE_ROOT/run" "$STATE_ROOT/result"

run_mode="${RUN_MODE:-normal}"
context_sha="${HEAD_SHA:-$(git -C /workspace rev-parse HEAD)}"
prompt_file="$STATE_ROOT/run/prompt.txt"
events_file="$STATE_ROOT/run/events.txt"
final_file="$STATE_ROOT/result/final.md"
meta_file="$STATE_ROOT/run/meta.json"

cat > "$meta_file" <<EOF
{
  "ai_tool": "cursor-cli",
  "pr_number": "${PR_NUMBER}",
  "pr_url": "${PR_URL:-}",
  "branch_name": "${PR_BRANCH:-}",
  "head_sha": "${context_sha}",
  "run_mode": "${run_mode}",
  "trigger_actor": "${TRIGGER_ACTOR:-}",
  "run_started_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
EOF

cat > "$prompt_file" <<EOF
あなたは GitHub Actions 上の Cursor CLI エージェントです。作業ディレクトリは /workspace です。

## 概要

この run は task スキルに沿って進めてください。ただし、依頼内容がタスク（実装や調査など）ではなく、単なる雑談や技術的な相談である場合は、この限りではありません。その場合は、task スキルを使用せず、自然な対話形式で返答してください。

**いちどの実行で全てのロードマップを完璧に引いたり、ステップを全て実行する必要は一切ありません**
自律レベル次第ですが、適切なタイミングでユーザーにレスポンスを返して動作を終了し、ユーザーに手戻りの指示をさせないようなやり方で進めてください。
作業の途中で該当ステップを完了したら、PR 本文に対応する見出しを追記・更新してください。

## 今回のタスクに関する情報

対象リポジトリ: ${REPO}
対象 PR 番号: ${PR_NUMBER}
対象 PR URL: ${PR_URL:-}
対象ブランチ: ${PR_BRANCH:-}
対象 head SHA: ${context_sha}
実行者: ${TRIGGER_ACTOR:-unknown}

## 利用可能な前提
- gh コマンドで PR 本文・コメント・差分・状態の参照や更新ができます
- git コマンドで編集、commit、push ができます
- 前回の会話（＝前回のGitHub Action 発火時）の際にあなた（＝AI）が退避させておいたファイルがある場合、それらは /state ディレクトリ配下に置いてあります
- PR 本文はストック型の中間成果物、PR コメントはフロー型のやりとりに使ってください

## 必須ルール
- **[最重要] ユーザーへの返事は、フロー型の情報として、すべてPRのコメント上で行ってください（メンションは不要です）**
- 依頼は task スキルに沿って進めてください
- **中間成果物（背景・目的、要件定義、設計、実装計画 など）は、すべてPRの概要欄を用いて、ストック型の情報として、適宜追加・更新してください**
  - 注意点として、ロードマップはPRの概要欄ではなくPRコメントとしてユーザーに提示してください
- 初回の依頼文言は PR コメントにあります。そこから作業の指示を読み取ってください
  - issue側にも追加でコメントがされている場合もあるため、issueが紐づいていた場合はそちらも読み込んでください
- 今回の呼び出しは初回ではなく、同じ PR に対する 2 回目以降の可能性があります。まず PR 本文、PR コメント、/state 配下を確認して、前回までのやり取りを引き継いでください
- task スキルの自律レベルについて、「今回の指示」に含まれている場合や、既にユーザーから指示を受けていた場合はそれに従ってください
  - 特にそのような指示がなかった場合は「自律レベル2」で進めてください
    - 自律レベル2：あなたのロードマップの提案自体はユーザーの承認を得る必要があるが、その承認されたロードマップの範疇においては途中のユーザーへの確認はせずに作業を進める指針
- 何をすべきか分からない場合は、この PR をきっかけに発火した直近の GitHub Actions run のログを見て、これまでの試行錯誤を軽く把握してください
  - それでも不明なら、PR コメントで確認してください
- ユーザーへの報告や確認が必要なときは、task スキルのフォーマットに寄せてください
- ロードマップの各ステップで使用するスキルについて「すべてのステップでtaskスキルを使用する」や「どのステップでもスキルを一切使わない」といったようなことはやめてください
- 確認を求めるときは \`## ロードマップ提案\` を使い、各項目に \`- ステップ名\` と \`- 理由\` を付けてください
- 認証や権限で進められない場合も、分かった範囲を PR コメントに残してください
- 返答に含めなかったが、次回あなたが動く際に覚えておきたい補助情報がある場合、 /state ディレクトリ配下に残してください（ここにファイルを置いておけば次回の会話に引き継がれるため、あなたが忘れずに済みます）
- PR 上で参照できる本文やコメントは /state ディレクトリに残す必要はありません（ghコマンドで参照できるため）
- /state ディレクトリになにか情報を残した場合、PR コメントでユーザーに返信する際にその旨も報告してください

## セキュリティとプライバシーに関する厳守事項
- **API キー、認証トークン、パスワード、パスフレーズ、その他機密情報をいかなるファイル（/state 配下を含む）にも書き出さないこと。**
- 実行ログや PR コメント、本文にも、これらの機密情報を絶対に含めないこと。
- 作業中に一時的に機密情報を扱う必要がある場合は、メモリ上でのみ処理し、永続化しないこと。

## 出力フォーマット
### PR本文
- \`## 背景・目的\` や \`## 要件\` など、引いたロードマップのステップに対応したような見出しで適宜追加すること
- 中間生成物を記載する場合は、以下の形式に従うこと
  - 「AI主導開発」で発火するCIが中間生成物をPR本文に記載する場合は、各中間生成物を \`<details>\` / \`<summary>\` で折りたたみ可能な構造にしてください。
  - 実装計画・調査結果・設計メモ・検証ログ・判断理由・生成された差分要約など、長くなりやすい情報は原則として折りたたんで記載します。

### PRコメント
- **[最重要ルールの再掲] ユーザーへの返事は、フロー型の情報として、すべてPRのコメント上で行ってください（メンションは不要です）**
- PRコメントにロードマップを記載する場合は、\`<details>\` / \`<summary>\` で折りたたみ可能な構造にしてください。
EOF

if [ "$run_mode" = "docs_update" ]; then
  cat >> "$prompt_file" <<EOF

docs 更新モードの追加ルール:
- この run の対象 PR は「作業中 PR」ではなく、すでに main にマージ済みの元 PR です。
- 現在の checkout は main です。main へ直接 push しないでください。
- \`gh pr view ${PR_NUMBER}\`、\`gh pr diff ${PR_NUMBER}\`、\`git show ${context_sha}\`、docs 配下、関連する実装ファイルを読んで、docs 更新が必要か判断してください。
- docs 更新が不要、または判断不能な場合は、新規 PR を作らずに終了してください。元 PR へのコメント投稿は必須ではありません。
- docs 更新が必要な場合だけ、\`ai/docs-update-pr-${PR_NUMBER}\` を基本に docs 更新用ブランチを作成し、docs 配下を編集して commit / push し、新規 PR を作成してください。
- docs 更新 PR にはラベル \`AIドキュメント更新\` のみを付けてください。ラベルが存在しない場合は \`gh label create "AIドキュメント更新" --color BFD4F2 --description "AI によるドキュメント更新 PR"\` で作成してください。
- docs 更新 PR 本文には、元 PR URL、更新が必要と判断した理由、更新ファイル、AI による判断・更新であることを含めてください。
- docs 配下以外の変更は原則行わないでください。どうしても必要な場合は、作成する PR 本文で理由を明記してください。
- docs 更新 PR 自体がマージされたときの再帰発火は、\`AIドキュメント更新\` ラベルで workflow 側がスキップします。
- このモードでは、PR 本文を中間成果物として更新する必要はありません。作成した docs 更新 PR の本文をストック型の成果物として扱ってください。

今回の指示:
EOF
else
  cat >> "$prompt_file" <<EOF

今回の指示:
EOF
fi

cat "$INSTRUCTION_FILE" >> "$prompt_file"

if ! command -v agent >/dev/null 2>&1; then
  echo "agent command not found in PATH" >&2
  exit 1
fi

prompt="$(cat "$prompt_file")"
agent -p --force --model auto --output-format text --workspace /workspace "$prompt" | tee "$events_file"
cp "$events_file" "$final_file"
