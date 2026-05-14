#!/usr/bin/env bash
set -euo pipefail

if [ -z "${INPUT_PR_TITLE:-}" ]; then
  echo "INPUT_PR_TITLE is required" >&2
  exit 1
fi

if [ -z "${INPUT_REQUEST_TEXT:-}" ]; then
  echo "INPUT_REQUEST_TEXT is required" >&2
  exit 1
fi

if [ -z "${DEFAULT_BRANCH:-}" ]; then
  echo "DEFAULT_BRANCH is required" >&2
  exit 1
fi

timestamp="$(date +%Y%m%d%H%M%S)"
slug="$(printf '%s' "$INPUT_PR_TITLE" \
  | tr '[:upper:]' '[:lower:]' \
  | sed -E 's/[^a-z0-9]+/-/g; s/^-+//; s/-+$//; s/-{2,}/-/g')"
slug="${slug:-task}"
slug="${slug:0:40}"
branch_name="ai/${timestamp}-${slug}"

git config user.name "github-actions[bot]"
git config user.email "41898282+github-actions[bot]@users.noreply.github.com"

git switch -c "$branch_name"
git commit --allow-empty -m "AIタスク開始: $INPUT_PR_TITLE"
git push --set-upstream origin "$branch_name"

body_file="$(mktemp)"
comment_file="$(mktemp)"

: > "$body_file"

{
  printf '依頼内容:\n'
  printf '%s\n' "$INPUT_REQUEST_TEXT"
} > "$comment_file"

pr_url="$(gh pr create \
  --draft \
  --base "$DEFAULT_BRANCH" \
  --head "$branch_name" \
  --title "$INPUT_PR_TITLE" \
  --body-file "$body_file")"

pr_number="$(gh pr view "$pr_url" --json number --jq '.number')"

gh pr comment "$pr_number" --body-file "$comment_file" >/dev/null

{
  echo "pr_number=$pr_number"
  echo "pr_url=$pr_url"
  echo "branch_name=$branch_name"
} >> "$GITHUB_OUTPUT"

rm -f "$body_file" "$comment_file"
