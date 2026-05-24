#!/usr/bin/env bash
set -euo pipefail

# AI 実行状態（/state）の暗号化・復号を行うスクリプト

COMMAND="${1:-}"
SRC_PATH="${2:-}"
DEST_PATH="${3:-}"
PASSPHRASE="${4:-}"

usage() {
  echo "Usage:"
  echo "  $0 encrypt <src_dir> <output_gpg_file> <passphrase>"
  echo "  $0 decrypt <src_gpg_file> <output_dir> <passphrase>"
  exit 1
}

if [ -z "$COMMAND" ] || [ -z "$SRC_PATH" ] || [ -z "$DEST_PATH" ] || [ -z "$PASSPHRASE" ]; then
  usage
fi

case "$COMMAND" in
  encrypt)
    echo "Encrypting $SRC_PATH to $DEST_PATH..."
    tar -cz -C "$SRC_PATH" . | \
      gpg --symmetric --batch --yes --passphrase "$PASSPHRASE" --cipher-algo AES256 -o "$DEST_PATH"
    echo "Encryption successful."
    ;;
  decrypt)
    echo "Decrypting $SRC_PATH to $DEST_PATH..."
    mkdir -p "$DEST_PATH"
    gpg --decrypt --batch --yes --passphrase "$PASSPHRASE" "$SRC_PATH" | \
      tar -xz -C "$DEST_PATH"
    echo "Decryption successful."
    ;;
  *)
    usage
    ;;
esac
