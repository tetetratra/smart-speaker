#!/usr/bin/env bash
set -euo pipefail

if [ -z "${GITHUB_EVENT_NAME:-}" ]; then
  echo "GITHUB_EVENT_NAME is required" >&2
  exit 1
fi

instruction_file="${RUNNER_TEMP:-/tmp}/ai_instruction.txt"
: > "$instruction_file"

should_run="false"
trigger_actor=""
pr_number=""
skip_reason=""
request_body=""
ai_label_name="AI主導開発"
docs_update_label_name="AIドキュメント更新"
run_mode="normal"

case "$GITHUB_EVENT_NAME" in
  workflow_dispatch)
    should_run="true"
    pr_number="${INPUT_PR_NUMBER:-}"
    trigger_actor="${GITHUB_ACTOR:-}"
    ;;
  issue_comment)
    if ! jq -e '.issue.pull_request' "$GITHUB_EVENT_PATH" >/dev/null; then
      should_run="false"
      skip_reason="not_pr_comment"
    else
      pr_number="$(jq -r '.issue.number' "$GITHUB_EVENT_PATH")"
      trigger_actor="$(jq -r '.comment.user.login' "$GITHUB_EVENT_PATH")"
      trigger_actor_type="$(jq -r '.comment.user.type // ""' "$GITHUB_EVENT_PATH")"
      comment_body="$(jq -r '.comment.body' "$GITHUB_EVENT_PATH")"
      if [ "$trigger_actor_type" = "Bot" ] || [ "$trigger_actor" = "github-actions[bot]" ]; then
        should_run="false"
        skip_reason="bot_comment"
      else
        pr_json="$(gh pr view "$pr_number" --json number,url,title,headRefName,headRefOid,body,labels)"
        pr_body="$(printf '%s' "$pr_json" | jq -r '.body // ""')"
        pr_labels="$(printf '%s' "$pr_json" | jq -r '.labels[].name // empty')"
        pr_has_ai_label="false"
        if printf '%s\n' "$pr_labels" | grep -Fxq "$ai_label_name"; then
          pr_has_ai_label="true"
        fi
        first_line="$(printf '%s\n' "$comment_body" | head -n 1)"
        if [ "$pr_has_ai_label" = "true" ]; then
          should_run="true"
          if [[ "$first_line" =~ ^/ai($|[[:space:]].*) ]]; then
            first_line_rest="$(printf '%s' "$first_line" | sed -E 's#^/ai[[:space:]]*##')"
            remaining_lines="$(printf '%s\n' "$comment_body" | tail -n +2)"
            {
              printf '%s' "$first_line_rest"
              if [ -n "$remaining_lines" ]; then
                if [ -n "$first_line_rest" ]; then
                  printf '\n'
                fi
                printf '%s' "$remaining_lines"
              fi
            } > "$instruction_file"
            request_body="$(cat "$instruction_file")"
          else
            request_body="$comment_body"
            printf '%s' "$request_body" > "$instruction_file"
          fi
        else
          skip_reason="missing_ai_label"
        fi
      fi
    fi
    ;;
  pull_request)
    action="$(jq -r '.action // ""' "$GITHUB_EVENT_PATH")"
    merged="$(jq -r '.pull_request.merged // false' "$GITHUB_EVENT_PATH")"
    base_ref="$(jq -r '.pull_request.base.ref // ""' "$GITHUB_EVENT_PATH")"
    pr_number="$(jq -r '.pull_request.number // ""' "$GITHUB_EVENT_PATH")"
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
      run_mode="docs_update"
      request_body="$(
        cat <<EOF
main にマージされた PR を契機に、docs 更新要否を確認してください。

対象のマージ済み PR:
- PR 番号: ${pr_number}
- PR URL: $(jq -r '.pull_request.html_url // ""' "$GITHUB_EVENT_PATH")
- PR タイトル: $(jq -r '.pull_request.title // ""' "$GITHUB_EVENT_PATH")
- base branch: ${base_ref}
- head branch: $(jq -r '.pull_request.head.ref // ""' "$GITHUB_EVENT_PATH")
- merge commit SHA: $(jq -r '.pull_request.merge_commit_sha // ""' "$GITHUB_EVENT_PATH")

マージ済み PR の差分と docs 配下を読み、ドキュメント更新が必要な場合のみ docs 更新用ブランチを作成して新規 PR を作ってください。
EOF
      )"
      printf '%s' "$request_body" > "$instruction_file"
    fi
    ;;
  *)
    echo "unsupported event: $GITHUB_EVENT_NAME" >&2
    exit 1
    ;;
esac

if [ "$should_run" != "true" ]; then
  {
    echo "should_run=false"
    echo "skip_reason=$skip_reason"
    echo "run_mode=$run_mode"
    echo "instruction_file=$instruction_file"
  } >> "$GITHUB_OUTPUT"
  exit 0
fi

if [ -z "$pr_number" ]; then
  echo "failed to resolve pr_number" >&2
  exit 1
fi

if [ "$run_mode" = "docs_update" ]; then
  pr_json="$(gh pr view "$pr_number" --json number,url,title,headRefName,headRefOid,body,mergeCommit)"
else
  pr_json="$(gh pr view "$pr_number" --json number,url,title,headRefName,headRefOid,body)"
fi
pr_body="$(printf '%s' "$pr_json" | jq -r '.body // ""')"

if [ "$GITHUB_EVENT_NAME" = "workflow_dispatch" ] && [ -z "$request_body" ]; then
  for _ in $(seq 1 10); do
    request_body="$(
      gh api "/repos/${REPO}/issues/${pr_number}/comments?per_page=100" \
        --jq 'sort_by(.created_at) | .[0].body // empty'
    )"
    if [ -n "$request_body" ]; then
      break
    fi
    sleep 1
  done
  if [ -z "$request_body" ]; then
    request_body="$pr_body"
  fi
fi

if [ "$GITHUB_EVENT_NAME" = "issue_comment" ] && [ -z "$request_body" ]; then
  request_body="$pr_body"
  if [ -n "$request_body" ]; then
    printf '%s' "$request_body" > "$instruction_file"
  fi
fi

if [ "$(printf '%s' "$request_body" | tr -d '[:space:]')" = "" ]; then
  should_run="false"
  skip_reason="missing_request"
fi

if [ "$should_run" != "true" ]; then
  {
    echo "should_run=false"
    echo "skip_reason=$skip_reason"
    echo "run_mode=$run_mode"
    echo "instruction_file=$instruction_file"
  } >> "$GITHUB_OUTPUT"
  exit 0
fi

if [ "$GITHUB_EVENT_NAME" = "workflow_dispatch" ]; then
  printf '%s' "$request_body" > "$instruction_file"
fi

{
  echo "should_run=true"
  echo "run_mode=$run_mode"
  echo "pr_number=$(printf '%s' "$pr_json" | jq -r '.number')"
  echo "pr_url=$(printf '%s' "$pr_json" | jq -r '.url')"
  echo "pr_title=$(printf '%s' "$pr_json" | jq -r '.title')"
  if [ "$run_mode" = "docs_update" ]; then
    echo "branch_name=main"
    echo "head_sha=$(printf '%s' "$pr_json" | jq -r '.mergeCommit.oid // .headRefOid')"
  else
    echo "branch_name=$(printf '%s' "$pr_json" | jq -r '.headRefName')"
    echo "head_sha=$(printf '%s' "$pr_json" | jq -r '.headRefOid')"
  fi
  echo "trigger_actor=$trigger_actor"
  echo "instruction_file=$instruction_file"
} >> "$GITHUB_OUTPUT"
