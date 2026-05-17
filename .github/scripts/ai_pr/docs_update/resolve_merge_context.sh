#!/usr/bin/env bash
set -euo pipefail

should_run="false"
skip_reason=""
pr_number=""
pr_url=""
trigger_actor=""
merge_commit_sha=""
request_body=""
docs_update_label_name="AIドキュメント更新"

action="$(jq -r '.action // ""' "$GITHUB_EVENT_PATH")"
merged="$(jq -r '.pull_request.merged // false' "$GITHUB_EVENT_PATH")"
base_ref="$(jq -r '.pull_request.base.ref // ""' "$GITHUB_EVENT_PATH")"
pr_number="$(jq -r '.pull_request.number // ""' "$GITHUB_EVENT_PATH")"
pr_url="$(jq -r '.pull_request.html_url // ""' "$GITHUB_EVENT_PATH")"
pr_title="$(jq -r '.pull_request.title // ""' "$GITHUB_EVENT_PATH")"
head_ref="$(jq -r '.pull_request.head.ref // ""' "$GITHUB_EVENT_PATH")"
merge_commit_sha="$(jq -r '.pull_request.merge_commit_sha // ""' "$GITHUB_EVENT_PATH")"
trigger_actor="$(jq -r '.sender.login // ""' "$GITHUB_EVENT_PATH")"
pr_labels="$(jq -r '.pull_request.labels[].name // empty' "$GITHUB_EVENT_PATH")"

if [ "$action" != "closed" ]; then
  skip_reason="not_pr_closed"
elif [ "$merged" != "true" ]; then
  skip_reason="not_merged"
elif [ "$base_ref" != "main" ]; then
  skip_reason="not_main_base"
elif printf '%s\n' "$pr_labels" | grep -Fxq "$docs_update_label_name"; then
  skip_reason="docs_update_pr"
else
  should_run="true"
  request_body="$(
    cat <<EOF
main にマージされた PR を契機に、docs 更新要否を確認してください。

対象のマージ済み PR:
- PR 番号: ${pr_number}
- PR URL: ${pr_url}
- PR タイトル: ${pr_title}
- base branch: ${base_ref}
- head branch: ${head_ref}
- merge commit SHA: ${merge_commit_sha}

マージ済み PR の差分と docs 配下を読み、ドキュメント更新が必要な場合のみ docs 更新用ブランチを作成して新規 PR を作ってください。
EOF
  )"
fi

{
  echo "should_run=$should_run"
  echo "skip_reason=$skip_reason"
  echo "pr_number=$pr_number"
  echo "pr_url=$pr_url"
  echo "trigger_actor=$trigger_actor"
  echo "merge_commit_sha=$merge_commit_sha"
  echo "instruction_body<<EOF"
  printf '%s\n' "$request_body"
  echo "EOF"
} >> "$GITHUB_OUTPUT"
