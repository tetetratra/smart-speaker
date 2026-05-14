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

case "$GITHUB_EVENT_NAME" in
  workflow_dispatch)
    should_run="true"
    pr_number="${INPUT_PR_NUMBER:-}"
    trigger_actor="${INPUT_BOOTSTRAP_ACTOR:-${GITHUB_ACTOR:-}}"
    printf '%s' "${INPUT_BOOTSTRAP_INSTRUCTION:-}" > "$instruction_file"
    ;;
  issue_comment)
    if ! jq -e '.issue.pull_request' "$GITHUB_EVENT_PATH" >/dev/null; then
      should_run="false"
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
    echo "instruction_file=$instruction_file"
  } >> "$GITHUB_OUTPUT"
  exit 0
fi

if [ -z "$pr_number" ]; then
  echo "failed to resolve pr_number" >&2
  exit 1
fi

pr_json="$(gh pr view "$pr_number" --json number,url,title,headRefName,headRefOid)"

{
  echo "should_run=true"
  echo "pr_number=$(printf '%s' "$pr_json" | jq -r '.number')"
  echo "pr_url=$(printf '%s' "$pr_json" | jq -r '.url')"
  echo "pr_title=$(printf '%s' "$pr_json" | jq -r '.title')"
  echo "branch_name=$(printf '%s' "$pr_json" | jq -r '.headRefName')"
  echo "head_sha=$(printf '%s' "$pr_json" | jq -r '.headRefOid')"
  echo "trigger_actor=$trigger_actor"
  echo "instruction_file=$instruction_file"
} >> "$GITHUB_OUTPUT"
