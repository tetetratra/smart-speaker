#!/usr/bin/env bash
set -euo pipefail

# 以下のコマンドでContextの作成は完了している前提
# docker context create production --docker "host=ssh://<user>@<ip>"

CONTEXT_NAME="production"
COMPOSE_FILE="docker-compose.yml"

production_docker_host="$(
  docker context inspect "$CONTEXT_NAME" --format '{{ (index .Endpoints "docker").Host }}'
)"
if [[ "$production_docker_host" != ssh://* ]]; then
  echo "Docker context '$CONTEXT_NAME' の接続先はSSH形式である必要があります。" >&2
  exit 1
fi

# デプロイ元PCではなく、本番サーバー自身のLAN IPとTailscale IPを取得する。
RTC_ICE_ADVERTISE_IPS="$(
  ssh "$production_docker_host" '
    set -eu

    default_interface="$(ip -4 route show default | sed -n "1s/.* dev \([^ ]*\).*/\1/p")"
    if [ -z "$default_interface" ]; then
      echo "本番サーバーのデフォルトネットワークインターフェースを取得できません。" >&2
      exit 1
    fi

    lan_ip="$(ip -4 -o address show dev "$default_interface" scope global | sed -n "1s/.* inet \([^/]*\)\/.*/\1/p")"
    if [ -z "$lan_ip" ]; then
      echo "本番サーバーのLAN IPを取得できません。" >&2
      exit 1
    fi

    tailscale_ip="$(tailscale ip -4)"
    if [ -z "$tailscale_ip" ]; then
      echo "本番サーバーのTailscale IPを取得できません。" >&2
      exit 1
    fi

    printf "%s,%s\n" "$lan_ip" "$tailscale_ip"
  '
)"
export RTC_ICE_ADVERTISE_IPS

current_context="$(docker context show)"
restore_context() {
  docker context use "$current_context"
}
# restore_context が必ず実行されるように、EXITトラップを設定
trap restore_context EXIT

docker context use "$CONTEXT_NAME"
docker compose -f "$COMPOSE_FILE" up -d --build --remove-orphans
docker compose -f "$COMPOSE_FILE" logs -f server
