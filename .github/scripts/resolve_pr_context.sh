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
request_body=""
reaction_comment_id=""
ai_label_name="AI主導開発"

is_write_collaborator() {
  local login="$1"
  local permission

  if [ -z "$login" ]; then
    return 1
  fi

  if ! permission="$(gh api "/repos/${REPO}/collaborators/${login}/permission" --jq '.permission' 2>/dev/null)"; then
    return 1
  fi

  case "$permission" in
    admin|maintain|write)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

comment_has_bot_reaction() {
  local comment_id="$1"
  local content="$2"

  gh api "/repos/${REPO}/issues/comments/${comment_id}/reactions?content=${content}&per_page=100" \
    --jq 'any(.[]; .user.type == "Bot" or .user.login == "github-actions[bot]" or .user.login == "github-actions")' \
    2>/dev/null
}

first_authorized_thumbs_up_user() {
  local comment_id="$1"
  local reactions_json
  local login

  if ! reactions_json="$(gh api "/repos/${REPO}/issues/comments/${comment_id}/reactions?content=%2B1&per_page=100" 2>/dev/null)"; then
    return 1
  fi

  while IFS= read -r login; do
    if is_write_collaborator "$login"; then
      printf '%s' "$login"
      return 0
    fi
  done < <(printf '%s' "$reactions_json" | jq -r '.[] | select(.user.type != "Bot") | .user.login')

  return 1
}

case "$GITHUB_EVENT_NAME" in
  workflow_dispatch)
    should_run="true"
    pr_number="${INPUT_PR_NUMBER:-}"
    trigger_actor="${GITHUB_ACTOR:-}"
    ;;
  schedule)
    while IFS= read -r candidate_pr_number; do
      comments_json="$(
        gh api "/repos/${REPO}/issues/${candidate_pr_number}/comments?per_page=100" \
          --jq 'sort_by(.created_at) | reverse'
      )"

      while IFS= read -r encoded_comment; do
        comment_json="$(printf '%s' "$encoded_comment" | base64 -d)"
        comment_id="$(printf '%s' "$comment_json" | jq -r '.id')"
        comment_body="$(printf '%s' "$comment_json" | jq -r '.body // ""')"

        if printf '%s\n' "$comment_body" | grep -Eq '^[[:space:]]*(依頼文言:|Codex 認証情報が未設定です。|AI 実行に失敗しました。)'; then
          continue
        fi

        if [ "$(comment_has_bot_reaction "$comment_id" "eyes")" = "true" ]; then
          continue
        fi

        if reactor="$(first_authorized_thumbs_up_user "$comment_id")"; then
          should_run="true"
          pr_number="$candidate_pr_number"
          trigger_actor="$reactor"
          reaction_comment_id="$comment_id"
          {
            printf 'PR 上の AI コメントに %s が +1 リアクションしました。\n' "$reactor"
            printf 'これは、対象コメントに対する「OKです」「了承します」と同等の返答として扱ってください。\n\n'
            printf '対象AIコメント:\n'
            printf '%s' "$comment_body"
          } > "$instruction_file"
          request_body="$(cat "$instruction_file")"
          break 2
        fi
      done < <(
        printf '%s' "$comments_json" \
          | jq -r '.[] | select(.user.type == "Bot") | @base64'
      )
    done < <(
      gh pr list --state open --label "$ai_label_name" --limit 100 --json number --jq '.[].number'
    )

    if [ "$should_run" != "true" ]; then
      skip_reason="no_pending_reaction"
    fi
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
        elif [[ "$first_line" =~ ^/ai($|[[:space:]].*) ]]; then
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

if [ -n "$autonomy_level_from_body" ]; then
  autonomy_level="$autonomy_level_from_body"
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
  echo "instruction_file=$instruction_file"
  echo "reaction_comment_id=$reaction_comment_id"
} >> "$GITHUB_OUTPUT"
