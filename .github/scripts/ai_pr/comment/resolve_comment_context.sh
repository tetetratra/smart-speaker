#!/usr/bin/env bash
set -euo pipefail

if [ -z "${REPO:-}" ]; then
  echo "REPO is required" >&2
  exit 1
fi

should_run="false"
skip_reason=""
pr_number=""
pr_url=""
branch_name=""
head_sha=""
trigger_actor=""
comment_id=""
request_body=""
ai_label_name="AI主導開発"

if ! jq -e '.issue.pull_request' "$GITHUB_EVENT_PATH" >/dev/null; then
  skip_reason="not_pr_comment"
else
  pr_number="$(jq -r '.issue.number' "$GITHUB_EVENT_PATH")"
  trigger_actor="$(jq -r '.comment.user.login // ""' "$GITHUB_EVENT_PATH")"
  comment_id="$(jq -r '.comment.id // ""' "$GITHUB_EVENT_PATH")"
  trigger_actor_type="$(jq -r '.comment.user.type // ""' "$GITHUB_EVENT_PATH")"
  comment_body="$(jq -r '.comment.body // ""' "$GITHUB_EVENT_PATH")"

  if [ "$trigger_actor_type" = "Bot" ] || [ "$trigger_actor" = "github-actions[bot]" ]; then
    skip_reason="bot_comment"
  else
    pr_json="$(gh pr view "$pr_number" --json number,url,title,state,headRefName,headRefOid,body,labels)"
    pr_state="$(printf '%s' "$pr_json" | jq -r '.state')"
    pr_url="$(printf '%s' "$pr_json" | jq -r '.url')"
    branch_name="$(printf '%s' "$pr_json" | jq -r '.headRefName')"
    head_sha="$(printf '%s' "$pr_json" | jq -r '.headRefOid')"
    pr_body="$(printf '%s' "$pr_json" | jq -r '.body // ""')"
    pr_labels="$(printf '%s' "$pr_json" | jq -r '.labels[].name // empty')"

    if [ "$pr_state" != "OPEN" ]; then
      skip_reason="closed_pr"
    elif ! printf '%s\n' "$pr_labels" | grep -Fxq "$ai_label_name"; then
      skip_reason="missing_ai_label"
    else
      request_body="$comment_body"

      if [ "$(printf '%s' "$request_body" | tr -d '[:space:]')" = "" ]; then
        request_body="$pr_body"
      fi

      if [ "$(printf '%s' "$request_body" | tr -d '[:space:]')" = "" ]; then
        skip_reason="missing_request"
      else
        should_run="true"
      fi
    fi
  fi
fi

{
  echo "should_run=$should_run"
  echo "skip_reason=$skip_reason"
  echo "pr_number=$pr_number"
  echo "pr_url=$pr_url"
  echo "branch_name=$branch_name"
  echo "head_sha=$head_sha"
  echo "trigger_actor=$trigger_actor"
  echo "comment_id=$comment_id"
  echo "instruction_body<<EOF"
  printf '%s\n' "$request_body"
  echo "EOF"
} >> "$GITHUB_OUTPUT"
