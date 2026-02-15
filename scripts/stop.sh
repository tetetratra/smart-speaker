#!/usr/bin/env bash
set -euo pipefail

CONTEXT_NAME="production"
COMPOSE_FILE="docker-compose.yml"

current_context="$(docker context show)"
restore_context() {
  docker context use "$current_context" >/dev/null 2>&1 || true
}
# restore_context が必ず実行されるように、EXITトラップを設定
trap restore_context EXIT

docker context use "$CONTEXT_NAME"
docker compose -f "$COMPOSE_FILE" down
