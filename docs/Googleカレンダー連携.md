# 7. Googleカレンダー連携

元ページ: https://www.notion.so/31db3ffbf12e81f896aefa627ced5fa5

## 1. ビジネスコンテキスト
- **目的**: ユーザーの予定を音声会話の中で参照・作成・更新できるようにする。
- **価値**: assistant が予定を知った上で自然に会話でき、必要なときはそのまま予定操作まで一貫して行える。
- **現在の設計方針**: OAuth / token 管理、Google Calendar API 呼び出し、会話文脈注入、ツール実行を分離しつつ、Calendar API 呼び出し自体は共通 `CalendarClient` に集約する。

## 2. 役割の分割
### 認証と token 管理
- Google OAuth 開始
- callback で token 交換
- refresh token を永続ファイルへ保存
- access token は必要時のみ refresh する

### Calendar API 呼び出し
- `ListEvents`
- `CreateEvent`
- `UpdateEvent`
- `DeleteEvent`
- Authorization ヘッダ付与、JSON 変換、HTTP エラー整形を共通化する

### GET キャッシュ
- `ListEvents` にだけ in-memory cache を持つ
- TTL は 5 分
- `CreateEvent` / `UpdateEvent` / `DeleteEvent` 成功時はキャッシュを全破棄する

### 利用側
- `conversation` は `ListEvents` を使って system context を組み立てる
- `google_calendar_list/create/update` ツールも同じ `CalendarClient` を使う

## 3. 論理構造・機能俯瞰
**主要なモデル・コンポーネント**
- **OAuth ハンドラ**
  - `/oauth/google/start`
  - `/oauth/google/callback`
  - `/oauth/google/status`
- **token store**
  - refresh token を永続ファイルに保存する
  - access token は必要に応じて refresh する
- **`CalendarClient`**
  - Google Calendar REST 呼び出しの共通クライアント
  - GET キャッシュを持つ
  - mutate 成功時にキャッシュを破棄する
- **calendar tools**
  - `google_calendar_list`
  - `google_calendar_create`
  - `google_calendar_update`
  - いずれも thin adapter として `CalendarClient` を呼ぶ
- **conversation context provider**
  - 会話 request の先頭に calendar prompt を system message として付与する
  - 自前では Calendar API を叩かず、shared `CalendarClient` を使う

## 4. 主要なデータフロー
### シナリオ: 初回認証
1. ブラウザが `/oauth/google/start` を開く
2. サーバーが state を Cookie に入れ、Google 認可画面へ redirect する
3. callback で `code` と `state` を受ける
4. authorization code を token に交換する
5. token を永続ファイルへ保存する
6. 次回以降は保存済み token を読む

### シナリオ: 会話時に予定を文脈へ入れる
1. `conversation` runtime が `contextProvider` を通じて calendar context を要求する
2. `CalendarClient.ListEvents` が token を取得する
3. `ListEvents` が primary calendar の今日〜3日分を取る
4. 取得結果は 5 分間キャッシュされる
5. `conversation` が `[今日] / [明日] / [明後日]` 形式の prompt を作る
6. それを system message として会話の先頭に付ける

### シナリオ: 予定を変更した直後
1. `google_calendar_create` または `google_calendar_update` が `CalendarClient` を呼ぶ
2. Calendar API の mutate が成功する
3. `CalendarClient` が GET キャッシュを全破棄する
4. 次回 `conversation` や `google_calendar_list` が `ListEvents` を呼ぶと再フェッチされる

## 5. shared CalendarClient 設計
### 置き場所
- `internal/googlecalendar/`
- 理由: OAuth 専用の `internal/oauth/googlecalendar/` と責務を分けるため

### クライアントが持つもの
- `http.Client`
- token 取得関数
- GET キャッシュ
- TTL 設定

### キャッシュキー
- `calendarID`
- `timeMin`
- `timeMax`
- `singleEvents`
- `orderBy`
- `maxResults`
これにより、同じ期間・同じ条件の一覧取得だけを再利用する。

## 6. 会話文脈とのつながり
Google Calendar は単なる CRUD 機能ではなく、conversation の system context の一部として使われる。
現在の扱い:
- 会話 request 発行時に context provider が diary と calendar を system message 化する
- calendar 側は shared `CalendarClient` の 5 分キャッシュを使う
- create / update / delete 成功時は `CalendarClient` がキャッシュを破棄する
- `conversation` 自体は calendar 更新系 tool 名を知らない
この設計により、会話ごとの無駄な GET を避けつつ、予定変更後は次回取得で自然に反映される。

## 7. 詳細設計
### クラス設計
- `internal/oauth/googlecalendar/config.go`
  - OAuth 設定の環境変数読み込み
- `internal/oauth/googlecalendar/http_handlers.go`
  - start / callback / status エンドポイント
- `internal/oauth/googlecalendar/token_store.go`
  - token 永続化と refresh
- `internal/googlecalendar/client.go`
  - `ListEvents` / `CreateEvent` / `UpdateEvent` / `DeleteEvent`
  - HTTP 実行とエラー整形
- `internal/googlecalendar/cache.go`
  - `ListEvents` 用の in-memory cache
- `internal/googlecalendar/types.go`
  - Calendar Event の共通 DTO
- `internal/tools/functions/googlecalendar/tool.go`
  - tool 引数を DTO に変換し、client を呼ぶ
- `internal/components/conversation/context_provider.go`
  - shared client を使って calendar context を組み立てる
- `cmd/smart-speaker/main.go`
  - `CalendarClient` を 1 回生成し、conversation と registry の両方へ注入する

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
- Calendar API 呼び出しは `conversation` とツールで共通 client を共有する
- GET キャッシュは `ListEvents` にのみ適用する
- mutate 後 invalidate は `conversation` ではなく `CalendarClient` の責務である
- `google_calendar_update` ツールは tool の外形上 `update/delete` をまとめているが、内部では `UpdateEvent` / `DeleteEvent` に分かれる

## 9. 参照実装
- `internal/oauth/googlecalendar/config.go`
- `internal/oauth/googlecalendar/http_handlers.go`
- `internal/oauth/googlecalendar/token_store.go`
- `internal/googlecalendar/client.go`
- `internal/googlecalendar/cache.go`
- `internal/googlecalendar/types.go`
- `internal/tools/functions/googlecalendar/tool.go`
- `internal/components/conversation/context_provider.go`
- `cmd/smart-speaker/main.go`
