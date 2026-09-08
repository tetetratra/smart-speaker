# smart-speaker

スマートフォンのブラウザを音声入力・音声再生の端末として使う、Web ベースのスマートスピーカーアプリケーションです。
ユーザーがブラウザに話しかけると、マイク音声が Go サーバーへ送られます。
サーバーでは音声認識、LLM による応答生成、音声合成を実行します。
生成した音声はブラウザへ返送し、端末側で再生します。

サーバー側では、音声入力から応答再生までの会話処理を実行します。
会話処理はイベントの流れとして扱い、音声入力、応答生成、音声再生、ツール実行を順番に制御します。

- マイク音声からの発話区間の検出
- ユーザーが話し始めたときの、再生中または生成中の応答の停止
- ユーザー、アシスタント、ツール実行結果の履歴の保持
- LLM の出力に従った、発話、待機、ツール呼び出しの順次実行

会話中に外部サービスを呼び出すツール機能があります。
現在は次の操作に対応しています。

- Google Calendar の予定の取得・作成
- SwitchBot を使った家電操作
- Web 検索で最新情報の取得

## 開発環境

### 起動

```sh
docker compose build
docker compose up
```

`http://localhost:5173` に配信されます。

### メモリ機能

メモリ機能は、会話履歴から長期記憶候補を作成し、ローカル embedding server の検索結果を LLM の入力に追加します。
Docker Compose ではメモリ store を `/app/data/memories.json` に保存し、host 側の `/var/lib/smart-speaker/data` に永続化します。

主な設定は以下です。

- `OPENAI_MEMORY_MODEL`: メモリ候補作成に使う Responses API model。未指定時は `OPENAI_RESPONSES_MODEL` を使います。
- `MEMORY_STORE_PATH`: メモリ JSON file store の保存先。Compose 既定値は `/app/data/memories.json` です。
- `MEMORY_EMBEDDING_BASE_URL`: ローカル embedding server の base URL。Compose 既定値は `http://embedding:80` です。
- `MEMORY_EMBEDDING_MODEL`: embedding service の model-id。既定値は `intfloat/multilingual-e5-small` です。
- `MEMORY_SIMILARITY_THRESHOLD`: 検索と近似重複判定に使う類似度閾値。既定値は `0.7` です。
- `MEMORY_MAX_CONTEXT_MEMORIES`: LLM に注入する最大メモリ件数。既定値は `3` です。
- `MEMORY_MAX_TAGS`: メモリ候補 1 件あたりの最大 tag 数。既定値は `5` です。

## 本番環境

### 準備
予め開発環境側で `production` Docker context を作成してください。

```sh
docker context create production --docker "host=ssh://<user>@<本番サーバーのIP>"
```

## デプロイ

```sh
./scripts/deploy.sh
```
