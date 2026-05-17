#!/usr/bin/env bash
set -euo pipefail

sanitize_slug() {
  printf '%s' "$1" \
    | tr '[:upper:]' '[:lower:]' \
    | sed -E 's/[^a-z0-9]+/-/g; s/^-+//; s/-+$//; s/-{2,}/-/g'
}

if [ -z "${INPUT_ISSUE_NUMBER:-}" ]; then
  echo "INPUT_ISSUE_NUMBER is required" >&2
  exit 1
fi

if [ -z "${DEFAULT_BRANCH:-}" ]; then
  echo "DEFAULT_BRANCH is required" >&2
  exit 1
fi

issue_json="$(gh issue view "$INPUT_ISSUE_NUMBER" --json number,title,body,url,author)"
issue_number="$(printf '%s' "$issue_json" | jq -r '.number')"
issue_title="$(printf '%s' "$issue_json" | jq -r '.title')"
issue_author_login="$(printf '%s' "$issue_json" | jq -r '.author.login // empty')"
ai_label_name="AI主導開発"

slug="$(sanitize_slug "${INPUT_BRANCH_SLUG:-}")"
if [ -z "$slug" ]; then
  slug="$(sanitize_slug "$issue_title")"
fi
slug="${slug:-issue}"
slug="${slug:0:48}"
branch_name="ai/${issue_number}-${slug}"

if ! git check-ref-format --branch "$branch_name" >/dev/null 2>&1; then
  branch_name="ai/${issue_number}-issue"
fi

git config user.name "github-actions[bot]"
git config user.email "41898282+github-actions[bot]@users.noreply.github.com"

git switch -c "$branch_name"
git commit --allow-empty -m "AIタスク開始: $issue_title"
git push --set-upstream origin "$branch_name"

pr_url="$(gh pr create \
  --base "$DEFAULT_BRANCH" \
  --head "$branch_name" \
  --title "$issue_title" \
  --body "")"

pr_number="$(gh pr view "$pr_url" --json number --jq '.number')"

if ! gh label list --limit 1000 --json name --jq '.[].name' | grep -Fxq "$ai_label_name"; then
  gh label create "$ai_label_name" \
    --color "BFD4F2" \
    --description "AI 主導で扱う PR に付けるラベル" >/dev/null
fi

gh pr edit "$pr_number" --add-label "$ai_label_name" >/dev/null

if [ -n "$issue_author_login" ] && [ "$issue_author_login" != "github-actions[bot]" ]; then
  if ! gh pr edit "$pr_number" --add-reviewer "$issue_author_login" >/dev/null; then
    echo "warning: failed to add reviewer: $issue_author_login" >&2
  fi
fi

{
  echo "issue_number=$issue_number"
  echo "pr_number=$pr_number"
  echo "pr_url=$pr_url"
  echo "branch_name=$branch_name"
} >> "$GITHUB_OUTPUT"
