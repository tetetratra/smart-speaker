#!/usr/bin/env bash
set -euo pipefail

if [ -z "${STATE_ROOT:-}" ]; then
  echo "STATE_ROOT is required" >&2
  exit 1
fi

if [ -z "${PR_NUMBER:-}" ]; then
  echo "PR_NUMBER is required" >&2
  exit 1
fi

if [ -z "${REPO:-}" ]; then
  echo "REPO is required" >&2
  exit 1
fi

if [ -z "${INSTRUCTION_FILE:-}" ]; then
  echo "INSTRUCTION_FILE is required" >&2
  exit 1
fi

mkdir -p "$STATE_ROOT/run" "$STATE_ROOT/result"

git config --global --add safe.directory /workspace || true

run_mode="${RUN_MODE:-normal}"
context_sha="${HEAD_SHA:-$(git -C /workspace rev-parse HEAD)}"
prompt_file="$STATE_ROOT/run/prompt.txt"
events_file="$STATE_ROOT/run/events.jsonl"
final_file="$STATE_ROOT/result/final.md"
meta_file="$STATE_ROOT/run/meta.json"

cat > "$meta_file" <<EOF
{
  "pr_number": "${PR_NUMBER}",
  "pr_url": "${PR_URL:-}",
  "branch_name": "${PR_BRANCH:-}",
  "head_sha": "${context_sha}",
  "run_mode": "${run_mode}",
  "trigger_actor": "${TRIGGER_ACTOR:-}",
  "run_started_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
EOF

AI_AGENT_NAME="Codex" bash /workspace/.github/scripts/ai_pr/common/build_prompt.sh "$prompt_file"

git -C /workspace submodule update --init external/skills

skills_src="/workspace/external/skills/skills"
skills_dest="${CODEX_HOME}/skills"
if [ -d "$skills_src" ]; then
  rm -rf "$skills_dest"
  mkdir -p "$skills_dest"

  shopt -s nullglob
  for skill_dir in "$skills_src"/*; do
    [ -d "$skill_dir" ] || continue

    skill_name="$(basename "$skill_dir")"
    if [ "${#skill_name}" -eq 1 ]; then
      continue
    fi

    cp -R "$skill_dir" "$skills_dest/"
  done
fi

cmd=(
  codex exec
  --dangerously-bypass-approvals-and-sandbox
  --ignore-user-config
  --json
  --color never
  --output-last-message "$final_file"
  -C /workspace
)

cmd+=( - )

"${cmd[@]}" < "$prompt_file" | tee "$events_file"
