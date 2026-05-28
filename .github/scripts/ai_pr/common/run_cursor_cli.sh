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

if [ -z "${CURSOR_API_KEY:-}" ]; then
  echo "CURSOR_API_KEY is required" >&2
  exit 1
fi

mkdir -p "$STATE_ROOT/run" "$STATE_ROOT/result"

git config --global --add safe.directory /workspace || true
export PATH="$HOME/.local/bin:/root/.local/bin:/home/node/.local/bin:$PATH"

run_mode="${RUN_MODE:-normal}"
context_sha="${HEAD_SHA:-$(git -C /workspace rev-parse HEAD)}"
prompt_file="$STATE_ROOT/run/prompt.txt"
events_file="$STATE_ROOT/run/events.txt"
final_file="$STATE_ROOT/result/final.md"
meta_file="$STATE_ROOT/run/meta.json"

cat > "$meta_file" <<EOF
{
  "ai_tool": "cursor-cli",
  "pr_number": "${PR_NUMBER}",
  "pr_url": "${PR_URL:-}",
  "branch_name": "${PR_BRANCH:-}",
  "head_sha": "${context_sha}",
  "run_mode": "${run_mode}",
  "trigger_actor": "${TRIGGER_ACTOR:-}",
  "run_started_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
EOF

AI_AGENT_NAME="Cursor CLI エージェント" bash /workspace/.github/scripts/ai_pr/common/build_prompt.sh "$prompt_file"

git -C /workspace submodule update --init external/skills

skills_src="/workspace/external/skills/skills"
skills_dest="/workspace/.cursor/skills"
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

if ! command -v agent >/dev/null 2>&1; then
  echo "agent command not found in PATH" >&2
  exit 1
fi

prompt="$(cat "$prompt_file")"
agent -p --force --model "composer-2.5" --output-format text --workspace /workspace "$prompt" | tee "$events_file"
cp "$events_file" "$final_file"
