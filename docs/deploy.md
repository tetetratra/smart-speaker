# 本番環境へのデプロイ

本番サーバーへは、手元の PC から Docker の SSH 接続でイメージをビルド・起動します。

## 準備（初回だけ）

1. 本番サーバーを指す `production` という Docker context を作ります。

   ```sh
   docker context create production --docker "host=ssh://<user>@<本番サーバーのIP>"
   ```

2. リポジトリ直下に `config.yaml` を用意します。イメージに含めて本番へ送られます。無いとデプロイスクリプトは止まります。
3. シークレットをリポジトリ直下の `.envrc` に書き、`direnv allow` で許可します（項目は[セットアップ](setup.md#設定)を参照）。
4. 本番サーバーで `ip` コマンドと `tailscale ip -4` を使えるようにし、LAN IP はルーターの DHCP 予約などで固定します。

デプロイスクリプトは、本番サーバーの LAN IP と Tailscale IP を取得して `RTC_ICE_ADVERTISE_IPS` に渡します。本番用の IP を `config.yaml` に書く必要はありません。

メモリ・予約ジョブ・Google のトークン・呼びかけ必須モードの設定は、本番サーバーの `/var/lib/smart-speaker/data` に保存されます。バックアップやトークンの置き直しは、このディレクトリに対して行います。

WebRTC 用の UDP ポート `50000-50100` はホストへ公開しますが、ルーターでインターネット向けのポート転送はしないでください。
外出先からは、tailnet に参加した端末で本番サーバーの Tailscale アドレスへ接続します。

## デプロイ

リポジトリの本来の場所（git worktree ではなく、メインのチェックアウト）で、main を最新にしてから実行します。

```sh
direnv exec . ./scripts/deploy.sh
```

- `direnv exec` を通さないと、`.envrc` のシークレットがコンテナへ渡らず、サーバーが起動に失敗します。
- worktree から実行すると、Compose のプロジェクト名が変わり、本番でコンテナが二重に起動します。
- worktree には Git 管理外の `config.yaml` や `.envrc` が無いため、そもそも失敗します。
- スクリプトは最後にサーバーのログを流し続けます。`Container smart-speaker-server-1 Started` が出て、エラーが出ていなければ、止めてかまいません。

本番のコンテナを止めるときは `./scripts/stop.sh` を使います。
