# 7. Googleカレンダー連携

元ページ: https://www.notion.so/31db3ffbf12e81f896aefa627ced5fa5

## 1. ビジネスコンテキスト
- **目的**: ユーザーの予定を音声会話の中で参照・作成・更新できるようにする。
- **価値**: assistant が予定を知った上で自然に会話でき、必要なときはそのまま予定操作まで一貫して行える。
- **現在の設計方針**: OAuth / token 管理、Google Calendar API 呼び出し、会話文脈注入、ツール実行を分離しつつ、Calendar API 呼び出し自体は共通 `CalendarClient` に集約する。`main` で 1 つの `CalendarClient` を生成し、conversation と tool registry の両方に渡す。

## 2. 役割の分割
### 認証と token 管理
- `GET /oauth/google/start` で state を生成し、Cookie に保存して Google 認可 URL へ redirect する
- `GET /oauth/google/callback` で `state` と Cookie を照合し、authorization code を token に交換する
- `GET /oauth/google/status` で token ファイルの有無と expiry を JSON で返す
- `oauth2.Token` を永続ファイルへ保存する。保存先は `GOOGLE_OAUTH_TOKEN_PATH`、未指定時は `data/google_oauth_token.json`
- Google の再発行時に refresh token が返らない場合でも、既存 refresh token を維持して保存する
- access token は `CalendarClient` のリクエスト直前に `oauth2.TokenSource` 経由で取得し、更新があれば永続ファイルへ保存する

### Calendar API 呼び出し
- `ListEvents`
- `CreateEvent`
- `UpdateEvent`
- `DeleteEvent`
- Authorization ヘッダ付与、JSON 変換、HTTP エラー整形を `CalendarClient` に共通化する
- `calendarID` の省略時は `primary`、一覧取得の既定値は `singleEvents=true`、`orderBy=startTime`、`maxResults=20`

### GET キャッシュ
- `ListEvents` にだけ in-memory cache を持つ
- TTL は 5 分
- `CreateEvent` / `UpdateEvent` / `DeleteEvent` 成功時はキャッシュを全破棄する
- キャッシュされた `[]Event` は取得・保存時に clone し、呼び出し側の変更が cache に混ざらないようにする

### 利用側
- `conversation` は保存済み token がある場合だけ `ListEvents` を使って calendar system context を組み立てる
- `google_calendar_list/create/update` ツールも同じ `CalendarClient` を使う
- delete は独立 tool ではなく、`google_calendar_update` の `action=delete` として実行する

## 3. 論理構造・機能俯瞰
**主要なモデル・コンポーネント**
- **OAuth ハンドラ**
  - `/oauth/google/start`
  - `/oauth/google/callback`
  - `/oauth/google/status`
- **token store**
  - `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` を必須設定として読む
  - `GOOGLE_REDIRECT_URL` の既定値は `http://localhost:8081/oauth/google/callback`
  - `GOOGLE_OAUTH_SCOPE` の既定値は `https://www.googleapis.com/auth/calendar.events`
  - `oauth2.Token` をメモリ cache と永続ファイルで保持する
  - access token は必要に応じて refresh し、refresh token は失わないように merge する
- **`CalendarClient`**
  - Google Calendar REST 呼び出しの共通クライアント
  - GET キャッシュを持つ
  - mutate 成功時にキャッシュを破棄する
- **calendar tools**
  - `google_calendar_list`
  - `google_calendar_create`
  - `google_calendar_update`
  - いずれも thin adapter として `CalendarClient` を呼ぶ
  - `google_calendar_update` は `action=update` と `action=delete` を持つ
- **conversation context provider**
  - 会話 request に diary と calendar の system message を付与する
  - 自前では Calendar API を叩かず、shared `CalendarClient` を使う
  - Google OAuth token が未保存なら calendar context は付与しない

## 4. 主要なデータフロー
### シナリオ: 初回認証
1. `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` / 必要に応じて `GOOGLE_REDIRECT_URL` を設定してサーバーを起動する
2. ブラウザが `/oauth/google/start` を開く
3. サーバーがランダムな state を作り、`google_oauth_state` Cookie に 10 分 TTL で入れる
4. サーバーが `access_type=offline` と `approval_prompt=force` を付けた Google 認可 URL へ redirect する
5. callback で `code` と `state` を受ける
6. Cookie の state と query の state を照合する
7. authorization code を token に交換する
8. token を永続ファイルへ保存し、state Cookie を削除する
9. 次回以降は保存済み token を読む

### シナリオ: 会話時に予定を文脈へ入れる
1. `conversation` runtime が `contextProvider.WithSystemContexts` を通じて system context を組み立てる
2. `contextProvider` が保存済み token の有無を `LoadToken` で確認する。未認証なら calendar context は付与しない
3. 認証済みなら 8 秒 timeout 付き context で `CalendarClient.ListEvents` を呼ぶ
4. 取得範囲は local time の今日 0:00 から 3 日後 0:00 まで、`calendarID=primary`、`maxResults=30`
5. `ListEvents` の取得結果は 5 分間キャッシュされる
6. `conversation` が `[今日] / [明日] / [明後日]` 形式の prompt を作る。予定がない日は `予定なし` と出す
7. calendar context と diary context を system message として会話に付与する。先頭に重要 retry system message がある場合は、その前には挿入しない

### シナリオ: 予定を変更した直後
1. `google_calendar_create` または `google_calendar_update` が `CalendarClient` を呼ぶ
2. Calendar API の mutate が成功する
3. `CalendarClient` が GET キャッシュを全破棄する
4. 次回 `conversation` や `google_calendar_list` が `ListEvents` を呼ぶと再フェッチされる
5. `google_calendar_update` の `action=delete` でも `DeleteEvent` 成功後に同じくキャッシュを全破棄する

## 5. shared CalendarClient 設計
### 置き場所
- `internal/googlecalendar/`
- 理由: OAuth 専用の `internal/oauth/googlecalendar/` と責務を分けるため

### クライアントが持つもの
- `http.Client`
- access token 取得関数
- GET キャッシュ
- TTL 設定
- base URL
- 現在時刻関数

### キャッシュキー
- `calendarID`
- `timeMin`
- `timeMax`
- `singleEvents`
- `orderBy`
- `maxResults`
`timeMin` / `timeMax` は UTC の RFC3339 文字列で key 化する。これにより、同じ期間・同じ条件の一覧取得だけを再利用する。

## 6. 会話文脈とのつながり
Google Calendar は単なる CRUD 機能ではなく、conversation の system context の一部として使われる。
現在の扱い:
- 会話 request 発行時に context provider が calendar と diary を system message 化する
- 実際の挿入順は、通常は diary system message、calendar system message、元の会話メッセージの順になる
- 先頭に `**[重要]** ` で始まる retry system message がある場合は、それを最優先で保持する
- calendar context は token 未保存時や取得失敗時には付与されない。取得失敗時は log のみ出して会話を継続する
- calendar 側は shared `CalendarClient` の 5 分キャッシュを使う
- create / update / delete 成功時は `CalendarClient` がキャッシュを破棄する
- `conversation` 自体は calendar 更新系 tool 名を知らない
calendar prompt は `以下はGoogleカレンダー情報です。会話の参考にしてください。` で始まり、`[今日] / [明日] / [明後日]` に予定を分類する。時刻付き予定は `HH:MM-HH:MM タイトル`、終日予定は `終日 タイトル`、タイトルなしは `(タイトルなし)` として表示する。
この設計により、会話ごとの無駄な GET を避けつつ、予定変更後は次回取得で自然に反映される。

## 7. 詳細設計
### クラス設計
- `internal/oauth/googlecalendar/config.go`
  - OAuth 設定の環境変数読み込み
  - `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` は必須
  - `GOOGLE_REDIRECT_URL` / `GOOGLE_OAUTH_SCOPE` は任意
- `internal/oauth/googlecalendar/auth_flow.go`
  - 認可 URL 生成、ローカル callback server、ブラウザ起動用の補助関数
- `internal/oauth/googlecalendar/http_handlers.go`
  - start / callback / status エンドポイント
  - state Cookie 生成、callback 検証、HTML 結果表示
- `internal/oauth/googlecalendar/token_store.go`
  - token 永続化と refresh
  - refresh token 維持、`GOOGLE_OAUTH_TOKEN_PATH` 対応
- `internal/googlecalendar/client.go`
  - `ListEvents` / `CreateEvent` / `UpdateEvent` / `DeleteEvent`
  - HTTP 実行とエラー整形
  - `Authorization: Bearer ...` の付与
- `internal/googlecalendar/cache.go`
  - `ListEvents` 用の in-memory cache
  - TTL 判定、全破棄、clone
- `internal/googlecalendar/types.go`
  - Calendar Event の共通 DTO
- `internal/tools/functions/googlecalendar/tool.go`
  - tool 引数を DTO に変換し、client を呼ぶ
  - `date` / RFC3339 / YYYY-MM-DD の入力を扱う
  - `google_calendar_update` の `action=delete` で `DeleteEvent` を呼ぶ
- `internal/components/conversation/context_calendar_format.go`
  - calendar events を `[今日] / [明日] / [明後日]` の system prompt に整形する
- `internal/components/conversation/context_provider.go`
  - shared client を使って calendar context を組み立てる
  - token 未保存時は calendar context を付与しない
  - diary context も同じ provider で付与する
- `cmd/smart-speaker/main.go`
  - `CalendarClient` を 1 回生成し、conversation と registry の両方へ注入する
  - `/oauth/google/*` を HTTP mux に登録する
  - `time.Local` を `Asia/Tokyo` に設定する

### API設計
- `GET /oauth/google/start`
- `GET /oauth/google/callback`
- `GET /oauth/google/status`
- `GET https://www.googleapis.com/calendar/v3/calendars/{calendarId}/events`
- `POST https://www.googleapis.com/calendar/v3/calendars/{calendarId}/events`
- `PATCH https://www.googleapis.com/calendar/v3/calendars/{calendarId}/events/{eventId}`
- `DELETE https://www.googleapis.com/calendar/v3/calendars/{calendarId}/events/{eventId}`

## 8. 現在の設計上の重要ポイント
- 認証はブラウザ経由だが、認証結果はサーバー側の永続ファイルへ残す
- 永続化対象は refresh token 単体ではなく `oauth2.Token` 全体であり、refresh token は再保存時に維持する
- `GOOGLE_OAUTH_SCOPE` の既定値は書き込み可能な `https://www.googleapis.com/auth/calendar.events`
- Calendar API 呼び出しは `conversation` とツールで共通 client を共有する
- GET キャッシュは `ListEvents` にのみ適用する
- mutate 後 invalidate は `conversation` ではなく `CalendarClient` の責務である
- `google_calendar_update` ツールは tool の外形上 `update/delete` をまとめているが、内部では `UpdateEvent` / `DeleteEvent` に分かれる
- `google_calendar_list` は `date` または `time_min` / `time_max` の両方指定が必須
- `google_calendar_create` は `summary` / `start_time` / `end_time` が必須
- `google_calendar_update` の update は空文字で項目を clear する用途には向いていない。空文字は未指定として扱われる
- `contextProvider` は calendar 取得失敗を致命エラーにせず、log のみ出して会話を続ける

## 9. 参照実装
- `internal/oauth/googlecalendar/config.go`
- `internal/oauth/googlecalendar/auth_flow.go`
- `internal/oauth/googlecalendar/http_handlers.go`
- `internal/oauth/googlecalendar/token_store.go`
- `internal/googlecalendar/client.go`
- `internal/googlecalendar/cache.go`
- `internal/googlecalendar/types.go`
- `internal/tools/functions/googlecalendar/tool.go`
- `internal/components/conversation/context_calendar_format.go`
- `internal/components/conversation/context_provider.go`
- `cmd/smart-speaker/main.go`
- `internal/app/config.go`
- `README.md`
