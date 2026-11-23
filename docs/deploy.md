# 本番環境へのデプロイ

本番サーバーへは、手元の PC から Docker の SSH 接続でイメージをビルド・起動します。

## 準備（初回だけ）

1. 本番サーバーを指す `production` という Docker context を作ります。

   ```sh
   docker context create production --docker "host=ssh://<user>@<本番サーバーのIP>"
   ```

2. リポジトリ直下に `config.yaml` を用意します。イメージに含めて本番へ送られます。無いとデプロイスクリプトは止まります。
3. リポジトリ直下に、シークレットを書いた `.env` を用意します（[.env.sample](../.env.sample) をコピーして値を書く。[セットアップ](setup.md#設定)も参照）。無いとデプロイスクリプトは止まります。
4. 本番サーバーで `ip` コマンドと `tailscale ip -4` を使えるようにし、LAN IP はルーターの DHCP 予約などで固定します。

デプロイスクリプトは、本番サーバーの LAN IP と Tailscale IP を取得して `RTC_ICE_ADVERTISE_IPS` に渡します。本番用の IP を `config.yaml` に書く必要はありません。

メモリ・予約ジョブ・Google のトークン・呼びかけ必須モードの設定は、本番サーバーの `/var/lib/smart-speaker/data` に保存されます。バックアップやトークンの置き直しは、このディレクトリに対して行います。

WebRTC 用の UDP ポート `50000-50100` はホストへ公開しますが、ルーターでインターネット向けのポート転送はしないでください。
外出先からは、tailnet に参加した端末で本番サーバーの Tailscale アドレスへ接続します。

## デプロイ

リポジトリの本来の場所（git worktree ではなく、メインのチェックアウト）で、main を最新にしてから実行します。

```sh
./scripts/deploy.sh
```

- `docker compose` が `.env` を読んでシークレットをコンテナへ渡すので、direnv を通す必要はありません。
- worktree から実行すると、Compose のプロジェクト名が変わり、本番でコンテナが二重に起動します。
- worktree には Git 管理外の `config.yaml` や `.env` が無いため、そもそも失敗します。
- スクリプトは最後にサーバーのログを流し続けます。`Container smart-speaker-server-1 Started` が出て、エラーが出ていなければ、止めてかまいません。

本番のコンテナを止めるときは `./scripts/stop.sh` を使います。

## 外部サービスの認可と準備

- Google Calendar（`plugins.googlecalendar`）の認可は、リダイレクト URI に localhost を使います。Google の OAuth は localhost 以外のリダイレクト URI に HTTPS を求めるためです。本番サーバーで認可するときは、`ssh -L 8081:localhost:8081 <user>@<本番サーバー>` でポートを転送してから、手元のブラウザで `http://localhost:8081/oauth/google/start` を開きます。
- stdio の MCP サーバー（`command`）は、本番では Docker コンテナの中で動きます。使うコマンドをイメージに入れておく必要があります。
