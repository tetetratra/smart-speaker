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

if [ -z "${STATE_ROOT:-}" ]; then
  echo "STATE_ROOT is required" >&2
  exit 1
fi

artifact_name="codex-state-pr-${PR_NUMBER}"

mkdir -p "$STATE_ROOT"

artifact_id="$(
  gh api "/repos/${REPO}/actions/artifacts?name=${artifact_name}&per_page=100" \
    --jq '.artifacts
      | map(select(.expired == false))
      | sort_by(.created_at)
      | last
      | .id // empty'
)"

if [ -n "$artifact_id" ]; then
  zip_file="${RUNNER_TEMP:-/tmp}/state-artifact-${PR_NUMBER}.zip"
  gh api "/repos/${REPO}/actions/artifacts/${artifact_id}/zip" > "$zip_file"
  unzip -oq "$zip_file" -d "$STATE_ROOT"
  rm -f "$zip_file"
fi

mkdir -p \
  "$STATE_ROOT/memory" \
  "$STATE_ROOT/notes" \
  "$STATE_ROOT/scratch" \
  "$STATE_ROOT/run" \
  "$STATE_ROOT/result"

{
  echo "artifact_name=$artifact_name"
  if [ -n "$artifact_id" ]; then
    echo "artifact_found=true"
  else
    echo "artifact_found=false"
  fi
} >> "$GITHUB_OUTPUT"
