# Google Calendar 認証処理 再設計ドキュメント

## 1. 目的
- Google Calendar 連携で必要な認証処理の責務と流れを整理し、再設計時の判断材料を揃える。
- 対象は Google OAuth の開始、callback、token 保存、認証状態確認、必要な環境変数に限定する。
- Calendar tool 自体の引数仕様は本ドキュメントの対象外とする。

## 2. 対象範囲
- `GET /oauth/google/start`
- `GET /oauth/google/callback`
- `GET /oauth/google/status`
- OAuth token の永続化と更新
- Google OAuth に必要な環境変数

## 3. 現状の構成
**主要コンポーネント**

- **設定読み込み**
  - `internal/oauth/googlecalendar/config.go` が `GOOGLE_CLIENT_ID`、`GOOGLE_CLIENT_SECRET`、`GOOGLE_REDIRECT_URL`、`GOOGLE_OAUTH_SCOPE` を読み込む。
  - `GOOGLE_CLIENT_ID` と `GOOGLE_CLIENT_SECRET` は必須。
  - `GOOGLE_REDIRECT_URL` 未指定時は `http://localhost:8081/oauth/google/callback` を使う。
  - `GOOGLE_OAUTH_SCOPE` 未指定時は `https://www.googleapis.com/auth/calendar.events` を使う。

- **OAuth 開始**
  - `internal/oauth/googlecalendar/http_handlers.go` の `handleOAuthStart` が state を生成する。
  - state は `google_oauth_state` Cookie として `/oauth/google` 配下に保存される。
  - Cookie には `HttpOnly`、`SameSite=Lax`、10 分の `MaxAge` が設定される。
  - その後、Google の認可 URL に `302 Found` でリダイレクトする。

- **OAuth callback**
  - `handleOAuthCallback` が query の `state` と `code` を受け取る。
  - Cookie に保存した state と query の state を照合する。
  - 一致した場合のみ authorization code を token に交換する。
  - token 保存後、state Cookie を削除し、HTML を返す。

- **token 保存**
  - `internal/oauth/googlecalendar/token_store.go` が token のメモリ cache と永続ファイル保存を担当する。
  - 保存先は `GOOGLE_OAUTH_TOKEN_PATH`、未指定時は `data/google_oauth_token.json`。
  - 保存時は既存 token の refresh token を引き継ぐ。
  - アクセストークン更新後に内容差分があれば再保存する。

- **認証状態確認**
  - `handleOAuthStatus` が `authenticated` を JSON で返す。
  - token を読み込めた場合は `authenticated: true` とし、expiry があれば RFC3339 文字列で返す。

## 4. 主要データフロー
### シナリオ: OAuth 開始
1. クライアントが `GET /oauth/google/start` にアクセスする。
2. サーバーがランダムな state を生成する。
3. サーバーが `google_oauth_state` Cookie を設定する。
4. サーバーが Google 認可 URL を生成する。
5. サーバーが認可 URL へリダイレクトする。

### シナリオ: callback
1. Google から `GET /oauth/google/callback?state=...&code=...` が呼ばれる。
2. サーバーが query の `state` と `code` の有無を確認する。
3. サーバーが Cookie の state を読み出す。
4. state が一致した場合のみ token 交換を実行する。
5. 取得した token を永続化する。
6. state Cookie を削除する。
7. 成功結果を HTML で返す。

### シナリオ: token 利用と再保存
1. サーバーが保存済み token を読み込む。
2. `oauth2.TokenSource` 経由でアクセストークンを取得する。
3. 新しい token に refresh token が含まれない場合、既存 refresh token を引き継ぐ。
4. token 内容が変わっていれば永続ファイルへ再保存する。

### シナリオ: 認証状態確認
1. クライアントが `GET /oauth/google/status` にアクセスする。
2. サーバーが token 読み込み可否を確認する。
3. token があれば `authenticated: true` を返す。
4. expiry が設定されていれば `expiry` も返す。

## 5. 必要な環境変数
- `GOOGLE_CLIENT_ID`
  - 必須。
  - Google OAuth クライアント ID。

- `GOOGLE_CLIENT_SECRET`
  - 必須。
  - Google OAuth クライアントシークレット。

- `GOOGLE_REDIRECT_URL`
  - 任意。
  - 未指定時は `http://localhost:8081/oauth/google/callback`。

- `GOOGLE_OAUTH_SCOPE`
  - 任意。
  - 未指定時は `https://www.googleapis.com/auth/calendar.events`。

- `GOOGLE_OAUTH_TOKEN_PATH`
  - 任意。
  - token 保存先パス。未指定時は `data/google_oauth_token.json`。

## 6. 再設計で維持したい性質
- state を Cookie と query で照合してから token 交換すること。
- token 保存先を環境変数で切り替えられること。
- refresh token が再発行されない更新時でも、既存 refresh token を失わないこと。
- 認証状態確認 API から、少なくとも認証済みかどうかを取得できること。

## 7. 不明点
- token ファイルの削除や失効を行う API は、参照した範囲では確認できていない。
- 複数ユーザーや複数 Google アカウントを同時に扱う前提かどうかは、不明。
- callback 成功後の画面遷移要件は、不明。現状は完了 HTML を返している。

## 8. 参照元
- [README.md](/Users/kondo.daichi/p/smart-speaker/README.md)
- [internal/oauth/googlecalendar/config.go](/Users/kondo.daichi/p/smart-speaker/internal/oauth/googlecalendar/config.go)
- [internal/oauth/googlecalendar/auth_flow.go](/Users/kondo.daichi/p/smart-speaker/internal/oauth/googlecalendar/auth_flow.go)
- [internal/oauth/googlecalendar/http_handlers.go](/Users/kondo.daichi/p/smart-speaker/internal/oauth/googlecalendar/http_handlers.go)
- [internal/oauth/googlecalendar/token_store.go](/Users/kondo.daichi/p/smart-speaker/internal/oauth/googlecalendar/token_store.go)
- [internal/oauth/googlecalendar/token_store_test.go](/Users/kondo.daichi/p/smart-speaker/internal/oauth/googlecalendar/token_store_test.go)
- 旧資料: `git show HEAD^:docs/7.Googleカレンダー連携.md`
