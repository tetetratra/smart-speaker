# smart-speaker

スマートフォンのブラウザを音声入力・音声再生の端末として使う、Web ベースのスマートスピーカーアプリケーションです。
ユーザーがブラウザに話しかけると、マイク音声が Go サーバーへ送られます。
サーバーは音声をRealtime APIへ送り、返ってきたテキストをVOICEVOXで音声合成します。
入力・出力ともにWebRTCの音声トラックを使います。出力音声はWeb Audioの音量調整を通して再生し、マイクのエコー除去を有効にします。
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

- `OPENAI_REALTIME_MODEL`: 会話モデル。既定は `gpt-realtime-2.1`。推論量はAPIへ指定せず、APIのデフォルトを使います（現在の公式仕様では`low`）。
- `OPENAI_REALTIME_VAD`: 発話の区切り。既定は`semantic_vad`、`eagerness`は`auto`。比較用に`server_vad`を指定すると、従来の800ms無音で終了する方式へ戻せます。未対応の値は起動時にエラーにします。
- `OPENAI_REALTIME_TRANSCRIPTION_MODEL`: 履歴保存用の非同期文字起こし。既定は `gpt-4o-mini-transcribe`。音声理解とは別に課金されます。
- `OPENAI_REALTIME_ENDPOINT`: テスト用のWebSocket接続先。通常は未指定にします。
- `OPENAI_RESPONSES_MODEL`: 長い履歴の要約・メモリ抽出の既定モデル・検索に使用します。
- `TTS_PROVIDER`: 既定は `voicevox`。`VOICEVOX_ENDPOINT` は既定で `http://voicevox:50021`。 `elevenlabs` を明示すればElevenLabsも選択できます。
- メイン画面の「読み上げ音声」でVOICEVOX／ElevenLabsを切り替えられます。設定済みの候補だけ選択でき、生成中・再生待ちの音声は保持し、次に生成する音声から反映します。選択は全端末で共有し、サーバー再起動時は`TTS_PROVIDER`の値に戻ります。
- `GET /api/tts`で現在の選択と設定済み候補を取得し、`PUT /api/tts`に`{"provider":"voicevox"}`または`{"provider":"elevenlabs"}`を送ると切り替えられます。候補の利用可否は設定の有無を示し、外部サービスへの疎通を保証するものではありません。
- ComposeのVOICEVOXは正式安定版 `cpu-0.26.0` に固定し、実行環境に合うamd64 / arm64を自動選択します。
- `CONVERSATION_IDLE_TIMEOUT_SECONDS`: 会話終了までの無操作時間。既定は300秒、0で無効。

発話の区切りは既定でAPIのSemantic VADにより内容から判断します。`auto`は`medium`相当で、言いよどみなどで終了が曖昧な場合の最大待ち時間は4秒です。常に4秒待つ意味ではなく、推論量の設定とも別です。アプリはAPIの自動応答を無効にしたまま、入力確定と履歴補正の後に応答生成を要求します。
Semantic VADを使っても、音声開始の検知による現在の読み上げ停止は維持します。相槌や雑音で停止した読み上げを自動で再開する機能は含みません。
待機キューは容量無制限です。応答出力のキューは最新の世代だけを保持し、旧世代の待機データと後から届く旧結果を破棄します。
受信直後の接続部分でも現在世代を確認し、通常の文章分割・音声合成処理には世代の判断を持たせません。
実行中の旧音声合成は完了させるため、その一件分の待ち時間は残ります。

合成した24kHz・モノラルPCM16を20ms単位に分割し、送信タイミング調整と世代フィルターを通した後、48kHzのOpusへ変換してWebRTCで送ります。
送信コンポーネントは待機音声を内部に蓄積しません。末尾の20ms未満の音声は無音で埋めて送ります。
AIの吹き出しは返答全文ではなく、再生する読み上げ区間ごとに切り替わります。表示単位の検討と動作は[吹き出しの表示単位](docs/bubble-display.md)を参照してください。
割り込み時はサーバー側で旧世代の未送信音声を破棄します。ブラウザで受信済みの音声は短い残りを再生し切り、同じ音声トラックで次の応答を続けます。残留時間は通信状況・端末のバッファに依存します。
WebSocketでは音声データを送らず、送信した区間の表示通知と世代変更を送ります。表示と実際の再生には受信バッファ分のずれがあります。
端末ごとの音量（0〜1.5倍）は、ブラウザのWeb AudioのGainNodeで適用します。WebSocketの`webrtc.volume`ではサーバー側を1倍に固定し、二重の増幅を避けます。

会話履歴はメモリ上だけに保存します。生成テキストと、最後のフレームまで送信できた文章を区別します。
途中で止まった文章は履歴補正時に中断として扱い、送信完了通知との微小な時間差は許容します。
API接続は55分経過後の会話・送信の切れ目で更新し、切断時も保存済みテキストとメモリから文脈を復元します。
文字起こし未完了の音声や切断中の入力を完全に復元するものではありません。
会話終了時は履歴のスナップショットを切り離して長期メモリを抽出します。
ツールの開始済み・結果不明の操作は再接続で自動再実行しません。
ツール結果は呼び出し時の世代が古くても会話へ追加し、説明が必要なら現在の世代で応答を要求します。ユーザーの発話・入力確定待ちや別の応答生成中は、結果に対する追加指示も保持して待機します。必要な場合は口頭説明より先にホワイトボードを更新し、成功後に結果を説明するよう指示します。取消済み・説明済みの結果の扱いと表示の必要性はモデルが判断し、報告完了を厳密に追跡するものではありません。
各ツールは`tools.DefinitionWithResponsePolicy`で成功後の再応答（`OnSuccess`）と追加指示（`Instructions`）を設定できます。未指定は再応答有効、失敗時は設定によらず説明を促します。結果自体は設定によらずAPIへ渡します。ホワイトボードも再応答を有効にし、未説明の取得結果だけを説明し、不要な完了報告や同じ表示の再更新はしないよう指示します。無音ジョブの結果は、次の通常会話へ自発的な読み上げとして持ち込みません。
「ここをもっと詳しく書いて」などの追加表示も、モデルが`set_whiteboard`を選んで行えます。情報取得後の自動更新だけに利用を制限しません。
`wait_for_user`はRealtimeステージが登録・処理する会話制御専用ツールです。一般のツール実行器へ渡さず、履歴とAPIに待機の完了を記録して、再応答・音声合成・ホワイトボード更新を起動せずに次の入力を待ちます。呼び出しの重複は再実行せず、次の入力の応答要求を抑制するフラグも設けません。
会話の接続時と各応答の生成時、およびWeb検索の実行時に現在の日本時間と今日・明日・昨日の日付を渡します。サーバーのタイムゾーンや、接続中の日付変更に依存しません。
Web検索は検索ツールの呼び出しを必須とし、完了した`web_search_call`を確認できない回答や未完了の応答を採用しません。出典の日付・内容の正しさはモデルへの指示で確認するため、検索済みでも誤った情報が含まれる可能性は残ります。
SwitchBotのシーン名は登録済み候補を`enum`で渡し、実行前にも完全一致で検証します。引数検証で中止した場合は未実行と候補を返します。結果不明の操作を自動再実行する仕組みは追加しません。
会話の指示では、意味が明確な省略は補い、自分宛ての依頼が聞き取れない場合は短い確認を一つだけ返し、自分宛てではない背景音・独り言には待機を選ぶよう区別します。通常の返答は1〜2文を目安とし、確認への「うん」などを短いという理由だけで無視しないよう求めます。
受信音量表示は廃止し、発話検知はAPI内蔵VADに集約しています。再生音量はブラウザ側で調整できます。

設定の仕様はOpenAI公式の[Realtimeプロンプトガイド](https://developers.openai.com/api/docs/guides/voice-prompting)、[VADガイド](https://developers.openai.com/api/docs/guides/realtime-vad)、[Realtime APIリファレンス](https://developers.openai.com/api/reference/resources/realtime/client-events)、[Web検索ガイド](https://developers.openai.com/api/docs/guides/tools-web-search)を参照してください。

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

メモリ機能は、会話が終わるたびに保存済みメモリと会話の発話を LLM に渡して、メモリの追加・更新・削除を判断させます。保存済みの全メモリは毎回 LLM の入力に追加します。
Docker Compose ではメモリ store を `/app/data/memories.json` に保存し、host 側の `/var/lib/smart-speaker/data` に永続化します。
通常画面の「メモリ」ボタン、または `?ui=memory` / `/memory` から、その時点で保存されているメモリ一覧を確認し、メモリ文の追加・編集・削除ができます。
追加・編集時は、入力したメモリ文からタグを再計算して保存します。

主な設定は以下です。

- `OPENAI_MEMORY_MODEL`: メモリの追加・更新・削除の判断に使う Responses API model。未指定時は `OPENAI_RESPONSES_MODEL` を使います。
- `MEMORY_STORE_PATH`: メモリ JSON file store の保存先。Compose 既定値は `/app/data/memories.json` です。
- `MEMORY_MAX_TAGS`: メモリ 1 件あたりの最大 tag 数。既定値は `5` です。

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

### ツール呼び出し履歴

メイン画面の「ツール履歴」または`/?ui=tools`で、呼び出しと結果を対応付けて確認できます。実行中・成功・失敗、引数と返り値の全文を表示します。サーバーが最新200件をメモリに保持するため、画面の再読み込み後も参照できますが、サーバー再起動で消去されます。`GET /api/tool-history`からも取得できます。

### 呼びかけ必須モード

UIと`set_wake_mode`で、既定の呼びかけ不要モードから必須モードへ切り替えられます。必須モードでは、ローカルSTTの信頼度と、文頭付近と呼びかけの言葉（既定は「起きて」）との類似度を判定し、通過した発話のテキストだけをRealtimeへ渡します。呼びかけ後は最後の発話から1分間、呼びかけなしで会話でき、その間は音声をそのままRealtimeへ送ります。呼びかけだけの発話には、普段の口調で起きていることを短く伝えます。オン・オフは永続化します。呼びかけの言葉と判定のしきい値は、デプロイ時の環境変数（`WAKE_WORDS`など）で決めます。ローカルSTTの準備と精度上の制限は[呼びかけ必須モードの仕様](docs/wake-mode.md)を参照してください。

### 自然言語の予約ジョブ

「10分後に起こして」「平日8時に処理して。音声通知は不要」などと会話から登録できます。指定時間後・日時指定・5項目のcron式に対応し、指示と予定はファイルに保存します。実行時ごとにAIが自然言語の指示を解釈し、必要なツールを呼び出します。「予約」画面で確認・変更・削除できます。日時指定の予約は実行後に削除されます。`speak=false`の指示は音声合成へ送らず、読み上げません。保持範囲・失敗時の扱い・APIの引数は[予約ジョブの仕様](docs/automations.md)を参照してください。

### 会話で利用を確認するサブエージェント

簡単な質問はRealtimeエージェント自身で対応します。難しい質問・調査・複雑な作業では、会話で「サブエージェントを使って調べていい？」と確認し、了承を得てから`ask_subagent`を呼び出すようプロンプトで指示します。許可画面やサーバー側の許可制限は設けません。

サブエージェントには、自分が補助役であり、ユーザーとの会話はRealtimeエージェントが通訳・仲介することを毎回の初期指示で伝えます。返答は`summary`（口頭用の要点、1〜3文・200文字以内）と`details`（ホワイトボード用の補足、通常5〜10行以内、最大10行・各120文字）に分けます。Realtimeエージェントには要点だけを読み上げ、詳細を`set_whiteboard`で残すよう指示します。発話の選択や会話での了承確認はモデルのプロンプト方針です。

初回は`session_id`を空文字にし、返されたIDを次の呼び出しで指定すると同じ会話を継続します。親の会話履歴は自動転送しません。`OPENAI_SUBAGENT_MODEL`の既定は`gpt-5.4`、推論量は`high`で、外部ツールは付与しません。Responses APIとStructured Outputsに対応するモデルが必要です。

セッションは最新の利用から24時間、最大32件をメモリに保持し、サーバー再起動で失われます。API側への保存を有効にし、`previous_response_id`で継続します。[会話状態](https://developers.openai.com/api/docs/guides/conversation-state)と[Structured Outputs](https://developers.openai.com/api/docs/guides/structured-outputs?api-mode=responses)の公式仕様を使用しています。失敗時は直前に成功した会話から再開します。

### 手元のMacのClaude Codeに作業を頼む

資料の読み込みや調査、文章作成など時間のかかる作業を、手元のMacで動くClaude Codeに依頼できます（`ask_claude_code`）。依頼はすぐに受け付けられ、終わると結果を読み上げます。Claude Codeが操作の許可を求めたときは、声で許可するか聞かれます。セッションの一覧と会話の内容も声で確かめられ、同じセッションの続きを頼んだり、PCで `claude --resume` して続けたりできます。

Macでは `services/agent-bridge` を常駐させ、サーバーに `AGENT_BRIDGE_URL` と `AGENT_BRIDGE_TOKEN` を設定します。未設定なら無効です。仕組みと安全のための制限は[Claude Code連携](docs/tools/claude-code.md)を参照してください。
