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
autonomy_level="level1"
codex_model=""
request_body=""

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
      comment_body="$(jq -r '.comment.body' "$GITHUB_EVENT_PATH")"
      first_line="$(printf '%s\n' "$comment_body" | head -n 1)"
      if [[ "$first_line" =~ ^/ai($|[[:space:]].*) ]]; then
        should_run="true"
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
        skip_reason="not_ai_comment"
      fi
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
    echo "instruction_file=$instruction_file"
  } >> "$GITHUB_OUTPUT"
  exit 0
fi

if [ -z "$pr_number" ]; then
  echo "failed to resolve pr_number" >&2
  exit 1
fi

pr_json="$(gh pr view "$pr_number" --json number,url,title,headRefName,headRefOid,body)"
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

autonomy_level_from_body="$(printf '%s\n' "$request_body" | sed -n 's/^自律レベル:[[:space:]]*//p' | head -n 1)"
codex_model_from_body="$(printf '%s\n' "$request_body" | sed -n 's/^Codexモデル:[[:space:]]*//p' | head -n 1)"

if [ -n "$autonomy_level_from_body" ]; then
  autonomy_level="$autonomy_level_from_body"
fi

if [ -n "$codex_model_from_body" ]; then
  codex_model="$codex_model_from_body"
fi

if [ "$(printf '%s' "$request_body" | tr -d '[:space:]')" = "" ]; then
  should_run="false"
  skip_reason="missing_request"
fi

if [ "$should_run" != "true" ]; then
  {
    echo "should_run=false"
    echo "skip_reason=$skip_reason"
    echo "instruction_file=$instruction_file"
  } >> "$GITHUB_OUTPUT"
  exit 0
fi

if [ "$GITHUB_EVENT_NAME" = "workflow_dispatch" ]; then
  printf '%s' "$request_body" > "$instruction_file"
fi

{
  echo "should_run=true"
  echo "pr_number=$(printf '%s' "$pr_json" | jq -r '.number')"
  echo "pr_url=$(printf '%s' "$pr_json" | jq -r '.url')"
  echo "pr_title=$(printf '%s' "$pr_json" | jq -r '.title')"
  echo "branch_name=$(printf '%s' "$pr_json" | jq -r '.headRefName')"
  echo "head_sha=$(printf '%s' "$pr_json" | jq -r '.headRefOid')"
  echo "trigger_actor=$trigger_actor"
  echo "autonomy_level=$autonomy_level"
  echo "codex_model=$codex_model"
  echo "instruction_file=$instruction_file"
} >> "$GITHUB_OUTPUT"
