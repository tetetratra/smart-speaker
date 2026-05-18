# Google Calendar tools 再設計ドキュメント

## 1. 目的
- Google Calendar tool 群の現状責務と接続点を整理し、再設計時の前提を揃える。
- 対象は認証前提、予定一覧/作成/更新/削除、会話文脈注入、共通 client の補足に限定する。
- Google Calendar API 自体の一般仕様説明は対象外とする。

## 2. 対象範囲
- `google_calendar_list`
- `google_calendar_create`
- `google_calendar_update`
  - `action=update`
  - `action=delete`
- `internal/components/conversation/context_provider.go` による会話文脈注入
- `internal/googlecalendar/*` の共通 client
- `internal/oauth/googlecalendar/*` の認証前提

## 3. 前提条件
- Google Calendar tool 群は、保存済みの Google OAuth token がある前提で動く。
- token 取得フローは `/oauth/google/start`、`/oauth/google/callback`、`/oauth/google/status` で提供される。
- OAuth 設定として `GOOGLE_CLIENT_ID` と `GOOGLE_CLIENT_SECRET` が必須。
- token 保存先は `GOOGLE_OAUTH_TOKEN_PATH`、未指定時は `data/google_oauth_token.json`。
- 共通 client が access token を取得する際、token 更新があれば永続ファイルへ再保存される。

## 4. 主要コンポーネント
| コンポーネント | 役割 |
| --- | --- |
| `internal/oauth/googlecalendar` | OAuth 設定読み込み、認可 URL 生成、callback 処理、token 保存、access token 更新を担う。 |
| `internal/googlecalendar.Client` | Google Calendar REST API の共通 client。一覧取得、作成、更新、削除と一覧 cache を持つ。 |
| `internal/tools/functions/googlecalendar` | tool 引数を解釈し、共通 client を呼ぶ薄い adapter。 |
| `internal/components/conversation/context_provider.go` | 会話開始時に Google Calendar の予定を system message として注入する。 |
| `cmd/smart-speaker/main.go` | 共通 client を 1 回生成し、conversation と tool registry の両方へ注入する。 |

## 5. tool 一覧
### `google_calendar_list`
- 用途: 指定期間または指定日の予定一覧取得。
- 主な引数:
  - `calendar_id`
  - `date`
  - `time_min`
  - `time_max`
  - `max_results`
- 入力ルール:
  - `date` または `time_min` と `time_max` の組のどちらかが必須。
  - `time_min` / `time_max` は両方指定が必要。
  - `date` は `YYYY-MM-DD`。
  - `time_min` / `time_max` は RFC3339 または `YYYY-MM-DD`。
- 既定値:
  - `calendar_id` 未指定時は `primary`
  - `max_results` 未指定時は `20`
- 出力:
  - `calendar_id`
  - `time_min`
  - `time_max`
  - `items[]`

### `google_calendar_create`
- 用途: 予定作成。
- 主な引数:
  - `calendar_id`
  - `summary`
  - `description`
  - `location`
  - `start_time`
  - `end_time`
- 必須:
  - `summary`
  - `start_time`
  - `end_time`
- 日時ルール:
  - `start_time` / `end_time` は RFC3339 または `YYYY-MM-DD`。
  - `YYYY-MM-DD` の場合は終日予定として扱う。
- 既定値:
  - `calendar_id` 未指定時は `primary`

### `google_calendar_update`
- 用途: 予定更新または削除。
- 主な引数:
  - `action`
  - `calendar_id`
  - `event_id`
  - `summary`
  - `description`
  - `location`
  - `start_time`
  - `end_time`
- `action`:
  - 未指定時は `update`
  - `delete` 指定時は削除を実行する
- 必須:
  - `event_id`
- 更新ルール:
  - `update` の場合、更新項目を 1 つ以上指定しないとエラー。
  - 空文字は未指定として扱われる。空文字での項目クリア用途には向いていない。
- 既定値:
  - `calendar_id` 未指定時は `primary`

### 削除の扱い
- 独立した `google_calendar_delete` tool は存在しない。
- 削除は `google_calendar_update` の `action=delete` に集約されている。

## 6. 共通 client の責務
### 対象
- `internal/googlecalendar/client.go`
- `internal/googlecalendar/cache.go`
- `internal/googlecalendar/types.go`

### 主な責務
- Google Calendar API への HTTP リクエスト実行
- `Authorization: Bearer ...` ヘッダ付与
- JSON の encode / decode
- エラーレスポンス整形
- `ListEvents` の cache
- mutate 成功後の cache 無効化

### 提供メソッド
- `ListEvents`
- `CreateEvent`
- `UpdateEvent`
- `DeleteEvent`

### 一覧取得の既定挙動
- `calendar_id` 未指定時は `primary`
- `orderBy` 未指定時は `startTime`
- `singleEvents` は実質的に常に `true`
- `maxResults` 未指定時は `20`

### cache
- 対象は `ListEvents` のみ。
- TTL は 5 分。
- cache key は以下の組み合わせ:
  - `calendarID`
  - `timeMin`
  - `timeMax`
  - `singleEvents`
  - `orderBy`
  - `maxResults`
- `timeMin` / `timeMax` は UTC の RFC3339 文字列で key 化される。
- `CreateEvent` / `UpdateEvent` / `DeleteEvent` 成功時は cache を全破棄する。
- cache への保存時と返却時に `[]Event` を clone し、呼び出し側の変更が cache に混ざらないようにしている。

## 7. 会話文脈注入
### 役割
- 会話 request の直前に、Google Calendar の予定を system message として注入する。
- これは tool 呼び出しとは別経路であり、assistant が予定を参照しながら応答するための文脈補強である。

### 挙動
- `contextProvider.buildCalendarContext` が保存済み token の有無を `LoadToken` で確認する。
- token がない場合、calendar 文脈は注入しない。
- token がある場合、8 秒 timeout 付き context で `ListEvents` を呼ぶ。
- 取得範囲は当日 0:00 から 3 日後 0:00 まで。
- `calendar_id` は `primary`。
- `maxResults` は `30`。
- 取得した予定は `[今日]`、`[明日]`、`[明後日]` の 3 区分に整形される。
- 時刻付き予定は `HH:MM-HH:MM タイトル` 形式。
- 終日予定は `終日 タイトル` 形式。
- タイトルが空なら `(タイトルなし)` を使う。
- 該当予定がない日は `予定なし` を出す。

### 失敗時の扱い
- calendar 文脈の構築失敗は会話全体の失敗にしない。
- ログ出力だけ行い、会話は継続する。

### 挿入順
- `WithSystemContexts` は calendar 文脈を元の会話メッセージの前に付与する。
- ただし先頭に `**[重要]** ` で始まる retry 用 system message がある場合は、それを最優先で保持する。

## 8. 主要データフロー
### シナリオ: 認証済み状態で予定一覧を取得する
1. tool caller が `google_calendar_list` を実行する。
2. tool が引数を解釈し、`ListEventsRequest` を組み立てる。
3. 共通 client が access token を取得する。
4. 共通 client が Google Calendar API の `events` 一覧取得を呼ぶ。
5. 結果を tool 出力形式へ整形して返す。

### シナリオ: 予定を作成する
1. tool caller が `google_calendar_create` を実行する。
2. tool が日時文字列を `EventTime` に変換する。
3. 共通 client が `CreateEvent` を実行する。
4. 成功時、共通 client が一覧 cache を全破棄する。
5. 作成結果を tool 出力として返す。

### シナリオ: 予定を更新または削除する
1. tool caller が `google_calendar_update` を実行する。
2. `action=delete` なら `DeleteEvent`、それ以外は `UpdateEvent` を呼ぶ。
3. 成功時、共通 client が一覧 cache を全破棄する。
4. 更新または削除の結果を tool 出力として返す。

### シナリオ: 会話時に予定を文脈へ注入する
1. conversation が system context 構築を開始する。
2. token が無ければ calendar 文脈の付与を省略する。
3. token があれば共通 client の `ListEvents` を呼ぶ。
4. 取得結果を 3 日分の prompt に整形する。
5. system message として会話メッセージ列へ挿入する。

## 9. 再設計で維持したい性質
- conversation と tool が同じ共通 client を共有すること。
- 一覧取得 cache の無効化責務を client 側に寄せること。
- OAuth token 更新時に refresh token を失わないこと。
- 認証されていない場合、会話文脈注入を静かにスキップできること。
- 削除を含む更新系操作の入口を、少なくとも現状互換では `google_calendar_update` にまとめられること。

## 10. 不明点
- 複数ユーザーや複数 Google アカウントを同時に扱う前提かどうかは不明。
- `calendar_id` に `primary` 以外を実運用で使う想定がどの程度あるかは不明。
- 予定参加者、リマインダー、繰り返し予定など、現在の tool 群が未対応の拡張項目を再設計対象に含めるかは不明。
- 会話文脈へ注入する 3 日分という期間を今後も維持すべきかは不明。

## 11. 参照元
- [internal/tools/functions/googlecalendar/tool.go](/Users/kondo.daichi/p/smart-speaker/internal/tools/functions/googlecalendar/tool.go)
- [internal/googlecalendar/client.go](/Users/kondo.daichi/p/smart-speaker/internal/googlecalendar/client.go)
- [internal/googlecalendar/cache.go](/Users/kondo.daichi/p/smart-speaker/internal/googlecalendar/cache.go)
- [internal/googlecalendar/types.go](/Users/kondo.daichi/p/smart-speaker/internal/googlecalendar/types.go)
- [internal/oauth/googlecalendar/config.go](/Users/kondo.daichi/p/smart-speaker/internal/oauth/googlecalendar/config.go)
- [internal/oauth/googlecalendar/auth_flow.go](/Users/kondo.daichi/p/smart-speaker/internal/oauth/googlecalendar/auth_flow.go)
- [internal/oauth/googlecalendar/http_handlers.go](/Users/kondo.daichi/p/smart-speaker/internal/oauth/googlecalendar/http_handlers.go)
- [internal/oauth/googlecalendar/token_store.go](/Users/kondo.daichi/p/smart-speaker/internal/oauth/googlecalendar/token_store.go)
- [internal/components/conversation/context_provider.go](/Users/kondo.daichi/p/smart-speaker/internal/components/conversation/context_provider.go)
- [internal/components/conversation/context_calendar_format.go](/Users/kondo.daichi/p/smart-speaker/internal/components/conversation/context_calendar_format.go)
- [cmd/smart-speaker/main.go](/Users/kondo.daichi/p/smart-speaker/cmd/smart-speaker/main.go)
- 旧資料: `git show HEAD^:docs/7.Googleカレンダー連携.md`
