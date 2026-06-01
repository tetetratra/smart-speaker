# memory 概要理解ドキュメント

## 1. 役割

memory は、会話から抽出した長期記憶を保存・検索するための土台です。
現時点では JSON file に永続化する Store と、ローカル embedding server から embedding を取得する client を提供します。
メモリ候補生成、session reset hook での保存、LLM context への注入は後続の変更対象です。

## 2. Store

`internal/states/memory.Store` は、メモリ本文、タグ、embedding、作成・更新時刻を保存します。
`Upsert` は content 完全一致、タグ集合一致、embedding の cosine similarity による近似一致を使って重複を判定します。
`Search` は query embedding と保存済み embedding の cosine similarity を計算し、閾値と最大件数を適用して類似度順に返します。

## 3. EmbeddingClient

`internal/hooks/memory.EmbeddingClient` は Docker Compose 内の `embedding` service に HTTP request を送ります。
接続先は `http://embedding:80`、モデルは `intfloat/multilingual-e5-small` 固定です。
これらは現時点では環境変数で切り替えません。

HTTP 契約は OpenAI-compatible API ではなく、Hugging Face Text Embeddings Inference のネイティブ API です。

```http
POST /embed
Content-Type: application/json

{"inputs":"検索または保存対象のテキスト"}
```

response は `number[][]` として decode し、単一入力の結果として先頭行を `[]float64` に変換します。
client 生成時には疎通確認を行わず、embedding server 未起動や HTTP error は `Embed` 実行時の error として返します。

## 4. Docker Compose

`docker-compose.yml` の `embedding` service は `ghcr.io/huggingface/text-embeddings-inference:cpu-1.9` を使います。
起動 command で `--model-id intfloat/multilingual-e5-small` を指定し、モデル cache は named volume `embedding-model-cache` の `/data` に保存します。
ホスト公開 port は追加せず、`server` から Compose 内部 DNS の `embedding` service 名で接続します。

## 5. 参照元

- `internal/states/memory/store.go`
- `internal/hooks/memory/embedding_client.go`
- `docker-compose.yml`
