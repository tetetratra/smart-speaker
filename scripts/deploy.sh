#!/usr/bin/env bash
set -euo pipefail

# 以下のコマンドでContextの作成は完了している前提
# docker context create production --docker "host=ssh://<user>@<ip>"

CONTEXT_NAME="production"
COMPOSE_FILE="docker-compose.yml"

if [[ -z "${RTC_ICE_PRODUCTION_ADVERTISE_IPS:-}" ]]; then
  echo "RTC_ICE_PRODUCTION_ADVERTISE_IPS に本番サーバーのLAN IPとTailscale IPを設定してください。" >&2
  exit 1
fi

# 開発PCの設定を本番へ誤って渡さないよう、本番専用設定から明示的に割り当てる。
export RTC_ICE_ADVERTISE_IPS="$RTC_ICE_PRODUCTION_ADVERTISE_IPS"

current_context="$(docker context show)"
restore_context() {
  docker context use "$current_context"
}
# restore_context が必ず実行されるように、EXITトラップを設定
trap restore_context EXIT

docker context use "$CONTEXT_NAME"
docker compose -f "$COMPOSE_FILE" up -d --build --remove-orphans
docker compose -f "$COMPOSE_FILE" logs -f server
