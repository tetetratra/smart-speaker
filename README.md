# smart-speaker

スマートフォンのブラウザを音声入力・音声再生の端末として使う、Web ベースのスマートスピーカーアプリケーションです。
ユーザーがブラウザに話しかけると、マイク音声が Go サーバーへ送られます。
サーバーは音声をRealtime APIへ送り、返ってきたテキストをVOICEVOXで音声合成します。
入力はWebRTC、出力は世代ID付きPCM音声をWebSocketで送り、ブラウザ側のキューから再生します。
生成した音声はブラウザへ返送し、端末側で再生します。

サーバー側では、音声入力から応答再生までの会話処理を実行します。
会話処理はイベントの流れとして扱い、音声入力、応答生成、音声再生、ツール実行を順番に制御します。

- Realtime API内蔵VADによる発話区間・割り込みの検出
- ユーザーが話し始めたときの、再生中または生成中の応答の停止
- ユーザー、アシスタント、ツール実行結果の履歴の保持
- 応答テキストの文章分割・音声合成と、function callingによるツール実行

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

### Realtime APIによる会話

- `OPENAI_REALTIME_MODEL`: 会話モデル。既定は `gpt-realtime-2.1-mini`。
- `OPENAI_REALTIME_TRANSCRIPTION_MODEL`: 履歴保存用の非同期文字起こし。既定は `gpt-4o-mini-transcribe`。音声理解とは別に課金されます。
- `OPENAI_REALTIME_ENDPOINT`: テスト用のWebSocket接続先。通常は未指定にします。
- `OPENAI_RESPONSES_MODEL`: 長い履歴の要約・メモリ抽出の既定モデル・検索に使用します。
- `TTS_PROVIDER`: 既定は `voicevox`。`VOICEVOX_ENDPOINT` は既定で `http://voicevox:50021`。
- `CONVERSATION_IDLE_TIMEOUT_SECONDS`: 会話終了までの無操作時間。既定は300秒、0で無効。

発話の区切りはAPIのVADで判断します。アプリは入力確定と履歴補正の後に応答生成を要求します。
待機キューは容量無制限です。応答出力のキューは最新の世代だけを保持し、旧世代の待機データと後から届く旧結果を破棄します。
受信直後の接続部分でも現在世代を確認し、通常の文章分割・音声合成処理には世代の判断を持たせません。
実行中の旧音声合成は完了させるため、その一件分の待ち時間は残ります。

音声は24kHz・モノラルPCM16を20ms単位で送ります。
ブラウザは世代付きキューを持ち、停止通知や新世代の音声で旧音声を破棄・停止します。
WebRTCの出力音声トラックは新経路では使用しません。

会話履歴はメモリ上だけに保存します。生成テキストと、最後のフレームまで送信できた文章を区別します。
途中で止まった文章は履歴補正時に中断として扱い、送信完了通知との微小な時間差は許容します。
API接続は55分経過後の会話・送信の切れ目で更新し、切断時も保存済みテキストとメモリから文脈を復元します。
文字起こし未完了の音声や切断中の入力を完全に復元するものではありません。
会話終了時は履歴のスナップショットを切り離して長期メモリを抽出します。
ツールの開始済み・結果不明の操作は再接続で自動再実行しません。

### 外部APIを使わないローカル確認

```sh
npm ci
npm test
npm run build
GOCACHE=/tmp/smart-speaker-go-cache go test -race ./...
GOCACHE=/tmp/smart-speaker-go-cache go run ./cmd/local-verify-realtime
```

`http://127.0.0.1:18081` を開いて接続します。マイクの許可が必要です。
通常のアプリを、偽Realtime APIとVOICEVOX互換HTTPサービスに接続して起動します。
本物のAPIキーや家電操作は使いません。確認用音声は読み上げではなく短い合成音です。
別のターミナルで次を実行し、音が続いている間に二つ目を実行すると割り込みを再現できます。

```sh
curl -X POST http://127.0.0.1:18082/debug/utterance -d '{"text":"最初の会話"}'
curl -X POST http://127.0.0.1:18082/debug/utterance -d '{"text":"割り込み後の会話"}'
```

検証コマンドはループバックアドレスだけで起動し、終了時に一時メモリを削除します。
このデバッグ経路は本番アプリには登録しません。
実APIでの音声理解・VOICEVOXの読み上げ品質・端末スピーカーからマイクへの回り込みは別途確認が必要です。

### WebRTCのICE候補

`RTC_ICE_ADVERTISE_IPS` には、ブラウザから到達できるサーバーのIPv4アドレスをカンマ区切りで指定します。
ローカル開発では、開発PCのLAN IPを指定します。
未指定の場合は、Pionが実行環境から検出したアドレスだけを使用します。

指定できるのは、RFC 1918のプライベートIPv4アドレスと、Tailscaleが使用する `100.64.0.0/10` のIPv4アドレスです。
意図しない外部公開を防ぐため、グローバルIPv4アドレスとIPv6アドレスは受け付けません。

PionのAddress Rewrite Rulesにより、指定したアドレスは既存のICE candidateへ追加されます。

https://pkg.go.dev/github.com/pion/webrtc/v4#SettingEngine.SetICEAddressRewriteRules

### メモリ機能

メモリ機能は、会話履歴から長期記憶候補を作成し、保存済みの全メモリを LLM の入力に追加します。
Docker Compose ではメモリ store を `/app/data/memories.json` に保存し、host 側の `/var/lib/smart-speaker/data` に永続化します。
通常画面の「メモリ」ボタン、または `?ui=memory` / `/memory` から、その時点で保存されているメモリ一覧を確認し、メモリ文の追加・編集・削除ができます。
追加・編集時は、入力したメモリ文からタグと embedding を再計算して保存します。

主な設定は以下です。

- `OPENAI_MEMORY_MODEL`: メモリ候補作成に使う Responses API model。未指定時は `OPENAI_RESPONSES_MODEL` を使います。
- `MEMORY_STORE_PATH`: メモリ JSON file store の保存先。Compose 既定値は `/app/data/memories.json` です。
- `MEMORY_EMBEDDING_BASE_URL`: ローカル embedding server の base URL。Compose 既定値は `http://embedding:80` です。
- `MEMORY_EMBEDDING_MODEL`: embedding service の model-id。既定値は `intfloat/multilingual-e5-small` です。
- `MEMORY_SIMILARITY_THRESHOLD`: 保存時の近似重複判定に使う類似度閾値。既定値は `0.95` です。
- `MEMORY_MAX_TAGS`: メモリ候補 1 件あたりの最大 tag 数。既定値は `5` です。

### 現在地API

iPhoneのショートカットなどから、現在いる場所を任意の文字列で送信できます。
経度・緯度は扱わず、最後に受け取った場所とサーバーでの更新日時だけをメモリ上に保持します。
サーバー再起動時に消去され、初回更新前は `null` です。同じ場所を再送した場合も更新日時を更新します。
このAPIでは情報を保持するだけで、会話の応答にはまだ反映しません。

Goサーバーのポート（既定は8081）にリクエストを送信します。
既存APIと同様にアプリ独自の認証はないため、自宅LANまたはTailscale経由で利用してください。

```sh
curl -X POST http://localhost:8081/api/location \
  -H 'Content-Type: application/json' \
  -d '{"place":"home"}'

curl http://localhost:8081/api/location
```

どちらも成功時は200で、更新後は次の形式を返します。`updated_at` はUTCのRFC3339形式です。

```json
{"place":"home","updated_at":"2026-11-03T00:00:00Z"}
```

`place` は空文字・空白のみ・未指定・null・文字列以外を受け付けません。
不正なJSONや複数のJSON値は400、本文が4KiBを超える場合は413、GET・POST以外は405を返し、保存済みの情報は変更しません。
空でない文字列は前後の空白も含めてそのまま保存します。

iPhoneのショートカットでは「URLの内容を取得」に次を設定します。

- URL: `http://<サーバーのLANまたはTailscaleアドレス>:8081/api/location`
- 方法: POST
- 要求本文: JSON
- フィールド: `place`（テキスト）に `home` などの場所名

設定したショートカットをパーソナルオートメーションなどから実行します。

## 本番環境

### 準備
予め開発環境側で `production` Docker context を作成してください。

```sh
docker context create production --docker "host=ssh://<user>@<本番サーバーのIP>"
```

デプロイスクリプトは `production` Docker contextのSSH接続先で、本番サーバー自身のLAN IPとTailscale IPを取得します。
取得した値は `RTC_ICE_ADVERTISE_IPS` に割り当てられるため、本番用IPをデプロイ元PCへ設定する必要はありません。
本番サーバーでは、`ip` コマンドと `tailscale ip -4` を実行できる必要があります。
本番サーバーのLAN IPは、家庭用ルーターのDHCP予約などで固定してください。

Docker ComposeはWebRTC用UDPポート `50000-50100` をホストへ公開しますが、家庭用ルーターでインターネット向けのポート転送は行わないでください。
外出先からは、tailnetへ参加した端末で本番サーバーのTailscaleアドレスへ接続します。

https://tailscale.com/docs/how-to/connect-to-devices
https://docs.docker.com/reference/cli/docker/context/inspect/
https://tailscale.com/docs/reference/tailscale-cli

## デプロイ

```sh
./scripts/deploy.sh
```
