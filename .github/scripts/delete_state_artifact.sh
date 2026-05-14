#!/usr/bin/env bash
set -euo pipefail

if [ -z "${PR_NUMBER:-}" ]; then
  echo "PR_NUMBER is required" >&2
  exit 1
fi

if [ -z "${REPO:-}" ]; then
  echo "REPO is required" >&2
  exit 1
fi

artifact_name="codex-state-pr-${PR_NUMBER}"

artifact_ids="$(
  gh api "/repos/${REPO}/actions/artifacts?name=${artifact_name}&per_page=100" \
    --jq '.artifacts
      | map(select(.expired == false))
      | .[].id'
)"

if [ -z "$artifact_ids" ]; then
  exit 0
fi

while IFS= read -r artifact_id; do
  [ -n "$artifact_id" ] || continue
  gh api -X DELETE "/repos/${REPO}/actions/artifacts/${artifact_id}" >/dev/null
done <<< "$artifact_ids"
