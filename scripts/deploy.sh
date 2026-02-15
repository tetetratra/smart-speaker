#!/usr/bin/env bash
set -euo pipefail

# 以下のコマンドでContextの作成は完了している前提
# docker context create production --docker "host=ssh://<user>@<ip>"

CONTEXT_NAME="production"
COMPOSE_FILE="docker-compose.yml"

current_context="$(docker context show)"
restore_context() {
  docker context use "$current_context"
}
# restore_context が必ず実行されるように、EXITトラップを設定
trap restore_context EXIT

docker context use "$CONTEXT_NAME"
docker compose -f "$COMPOSE_FILE" up -d --build
docker compose -f "$COMPOSE_FILE" logs -f
