# memory hook 概要理解ドキュメント

## 1. ビジネスコンテキスト

- **解決する課題**: 会話から抽出した長期記憶を、後続の発話や検索に再利用できる土台を用意する
- **ターゲットユーザー**: smart-speaker を利用するユーザーと、メモリ機能を実装・保守する開発者
- **価値定義**: ローカル embedding server と JSON file store を使い、外部 embedding API に依存せずメモリ検索を組み立てられる

## 2. 論理構造・機能俯瞰

**主要なモデル・コンポーネント**

- **`internal/hooks/memory.OpenAIClient`**
  - reset 前の会話履歴を OpenAI Responses API に渡し、長期記憶候補を structured output として生成する
  - 候補は `content` と `tags[]` を持つ
  - 通常起動では `OPENAI_MEMORY_MODEL` を使い、未指定の場合は `OPENAI_RESPONSES_MODEL` に fallback する
  - 1 reset あたりの候補数は既定で最大 5 件、1 候補あたりの tag は既定で最大 5 件
  - 履歴が空の場合は OpenAI API を呼ばず、候補なしとして返す
- **`internal/hooks/memory.CreatorHook`**
  - `sessionreset.Hook` として reset 前に同期実行される
  - `conversationhistory.Store.Snapshot()` から reset 前履歴を読み、`OpenAIClient` でメモリ候補を生成する
  - 候補ごとに `content` と `tags` を連結した検索用文字列から embedding を生成し、`memory.Store.Upsert` に保存を委譲する
  - 候補生成全体に失敗した場合は error を返す
  - 候補単位の embedding / upsert 失敗は残り候補の処理を継続し、最後に error を集約して返す
- **`internal/hooks/memory.EmbeddingClient`**
  - Docker Compose 内の `embedding` service に HTTP request を送り、テキストから embedding を取得する
  - 既定の接続先は `http://embedding:80`、既定のモデルは `intfloat/multilingual-e5-small`
  - OpenAI-compatible API ではなく、Hugging Face Text Embeddings Inference のネイティブ `POST /embed` を使う
- **`internal/states/memory.Store`**
  - メモリ本文、タグ、embedding、作成・更新時刻を JSON file に永続化する
  - content 完全一致、タグ集合一致、embedding の cosine similarity で重複を判定する
  - query embedding と保存済み embedding の cosine similarity で検索する
- **`embedding` service**
  - `docker-compose.yml` で起動するローカル embedding server
  - host port は公開せず、Go server から Compose 内部 DNS で接続する

## 3. 設定

通常起動では、メモリ機能の設定を環境変数から読み込みます。

| 環境変数 | 既定値 | 用途 |
| --- | --- | --- |
| `OPENAI_MEMORY_MODEL` | `OPENAI_RESPONSES_MODEL` | reset 前履歴からメモリ候補を作る Responses API の model |
| `MEMORY_STORE_PATH` | `data/memories.json` | メモリ JSON file store の保存先 |
| `MEMORY_EMBEDDING_BASE_URL` | `http://embedding:80` | ローカル embedding server の base URL |
| `MEMORY_EMBEDDING_MODEL` | `intfloat/multilingual-e5-small` | Docker Compose の embedding service で起動する model |
| `MEMORY_SIMILARITY_THRESHOLD` | `0.7` | LLM 注入時の検索閾値と保存時の近似重複判定閾値 |
| `MEMORY_MAX_CONTEXT_MEMORIES` | `3` | LLM context に注入する最大メモリ件数 |
| `MEMORY_MAX_TAGS` | `5` | メモリ候補 1 件あたりの最大 tag 数 |

Docker Compose では `MEMORY_STORE_PATH=/app/data/memories.json`、`MEMORY_EMBEDDING_BASE_URL=http://embedding:80` を server に渡します。
`/app/data` は host の `/var/lib/smart-speaker/data` に volume mount されるため、メモリ JSON はコンテナ再作成後も保持されます。
embedding は OpenAI API ではなく、Compose 内の `embedding` service が提供する TEI ネイティブ API を使います。

## 4. 主要なデータフロー

### シナリオ: session reset 前の会話からメモリ候補を作成・保存する

1. reset 発火: `sessionreset` が idle timeout 到達時に登録済み hook を reset 前に実行する。
2. 履歴取得: `CreatorHook` が `conversationhistory.Store.Snapshot()` で reset 前履歴を取得する。
3. 候補生成: `OpenAIClient` が会話履歴を JSON 文字列として Responses API に送り、`content` と `tags[]` を持つ候補配列を受け取る。
4. 候補正規化: 空の `content` は除外し、`tags` は trim、空文字除外、大文字小文字を無視した重複除外、最大件数で切り詰める。
5. embedding 生成: `EmbeddingClient` が候補ごとの `content` と `tags` を連結した検索用文字列を `POST http://embedding:80/embed` に送る。
6. 保存: `memory.Store.Upsert` が `content`、`tags`、embedding を保存し、既存 record との重複を判定する。
7. reset 継続: hook が error を返しても、`sessionreset` の既存仕様により履歴 reset、世代更新、agent status idle 化は継続される。

```mermaid
sequenceDiagram
    participant SR as sessionreset
    participant Hook as CreatorHook
    participant Hist as conversationhistory.Store
    participant OpenAI as OpenAI Responses API
    participant Client as EmbeddingClient
    participant TEI as embedding service
    participant Store as memory.Store

    SR->>Hook: Exec(ctx)
    Hook->>Hist: Snapshot()
    Hook->>OpenAI: reset前履歴から candidates を生成
    OpenAI-->>Hook: [{content, tags}]
    loop candidate ごと
        Hook->>Client: Embed(ctx, content + tags)
        Client->>TEI: POST /embed {"inputs": "..."}
        TEI-->>Client: [[0.0123, -0.0456, ...]]
        Client-->>Hook: []float64
        Hook->>Store: Upsert(content, tags, embedding)
    end
    SR->>Hist: Reset()
```

### シナリオ: user 発話から関連メモリを検索して LLM に注入する

1. user 発話が履歴へ保存される。
2. `llm` が応答生成前に保存済み履歴 snapshot を読む。
3. `ContextProvider` が直近の user / agent / system 発話から検索 query を作る。
4. `EmbeddingClient` が query を `POST /embed` に送り、検索用 embedding を取得する。
5. `memory.Store.Search` が similarity threshold と最大件数を適用し、類似度順に結果を返す。
6. `llm` が検索結果の `content` だけを `memory_context` system message として通常の会話履歴より前に追加する。

```mermaid
sequenceDiagram
    participant LLM as llm
    participant Hist as conversationhistory.Store
    participant Provider as ContextProvider
    participant Client as EmbeddingClient
    participant TEI as embedding service
    participant Store as memory.Store
    participant OpenAI as OpenAI Responses API

    LLM->>Hist: Snapshot()
    LLM->>Provider: BuildContext(ctx, records)
    Provider->>Client: Embed(ctx, query)
    Client->>TEI: POST /embed {"inputs": "..."}
    TEI-->>Client: [[0.0123, -0.0456, ...]]
    Provider->>Store: Search(embedding, threshold, limit)
    Store-->>Provider: related memories
    Provider-->>LLM: memory_context system message
    LLM->>OpenAI: memory_context + conversation history
```

## 5. 失敗時の扱い

- メモリ候補作成で OpenAI Responses API が失敗した場合、`CreatorHook` は error を返しますが、`sessionreset` は error をログに残して reset 処理を継続します。
- 候補単位の embedding 生成または保存に失敗した場合、`CreatorHook` は残り候補の処理を継続し、最後に error を集約して返します。
- LLM 注入前の memory context 取得に失敗した場合、`llm` は error をログに残し、memory context なしで通常の応答生成を継続します。
- store file が読めない、または不正な version の JSON がある場合は通常起動時の store 初期化に失敗します。

## 6. 詳細設計

### クラス設計

- `internal/`
  - `hooks/`
    - `memory/`
      - `openai_client.go`: reset 前会話履歴からメモリ候補を生成する
        - `NewOpenAIClient`: API key、model、endpoint、HTTP client、最大候補数、最大 tag 数を受け取る
        - `CreateCandidates`: 履歴が空なら候補なしを返し、履歴があれば Responses API の strict JSON schema で候補を生成する
        - `memoryCandidateInstructions`: 長期記憶候補の抽出ルールと出力例を定義する
        - `normalizeCandidates`: 空 `content` の除外、`tags` の trim / 重複除外 / 件数制限を行う
      - `creator_hook.go`: session reset 前にメモリ候補を生成して保存する hook を担当する
        - `NewCreatorHook`: history、candidate creator、embedder、memory upserter を受け取り、必須依存の nil を拒否する
        - `Exec`: reset 前履歴の snapshot、候補生成、embedding 生成、Store upsert を順に実行する
        - 候補単位の embedding / upsert 失敗は `errors.Join` で集約しつつ、残り候補の処理を続ける
      - `embedding_client.go`: TEI `/embed` への HTTP 通信と `number[][]` response の変換を担当する
        - `NewEmbeddingClient`: base URL の default 補完と形式検証を行う。生成時の疎通確認はしない
        - `Embed`: 空 text を拒否し、HTTP error や空 embedding を error として返す
  - `states/`
    - `memory/`
      - `store.go`: メモリ record の永続化、重複判定、検索を担当する
        - `Upsert`: content、tags、embedding 類似度で重複を判定して保存する
        - `Search`: query embedding と保存済み embedding の cosine similarity で結果を返す

### API設計

- `POST https://api.openai.com/v1/responses`: reset 前会話履歴からメモリ候補を生成する
  - system input: 長期記憶候補の抽出ルールと出力例
  - user input: reset 前の `ConversationRecord` 配列を JSON 文字列化したもの
  - structured output schema: `{ "candidates": [{ "content": string, "tags": string[] }] }`
  - 空候補は `{ "candidates": [] }` として扱う
- `POST http://embedding:80/embed`: 単一テキストから embedding vector を取得する
  - リクエスト: `{"inputs":"検索または保存対象のテキスト"}`
  - レスポンス: `[[0.0123, -0.0456, 0.0789]]`

### メモリ候補の生成ルール

- `content` は 1 候補につき 1 つの再利用可能な事実を、短く、主語が分かる自然文で表す。
- `tags` は検索補助用の短いラベルとして扱う。
- `tags` は内部 key として使いやすいよう、英単語に寄せる。必要に応じて `smart_home` や `living_room` のような snake_case を使う。
- 会話ログの生コピー、一時的な依頼、古くなりやすい状態、秘密情報らしき内容は保存候補にしない。
- 保存すべき長期記憶候補がない場合は、空の候補配列を正常系として扱う。

例:

```json
{
  "candidates": [
    {
      "content": "ユーザーは平日の朝にコーヒーを飲むことが多い",
      "tags": ["routine", "morning", "coffee", "weekday"]
    },
    {
      "content": "ユーザーはリビングの照明操作に SwitchBot ハブミニを使っている",
      "tags": ["SwitchBot", "smart_home", "living_room", "lighting", "device"]
    }
  ]
}
```

### 現時点の対象外

- メモリを閲覧・編集・削除する UI
- メモリ store の migration
- embedding service の host port 公開
