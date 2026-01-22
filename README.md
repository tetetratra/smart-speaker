# Smart Speaker (Go) + WebSocket 音声 I/O

## 前提
- Go 1.24 以降
- Node 20 以降（フロント開発用）  
  ※ `npm install` でローカルに Vite が入ります（グローバルインストール不要）

## 環境変数
- `OPENAI_API_KEY`（必須）
- `ELEVENLABS_API_KEY` / `ELEVENLABS_VOICE_ID`（テキスト→音声を ElevenLabs で生成するために必須）
- `ELEVENLABS_MODEL_ID`（任意、デフォルト `eleven_multilingual_v2`）
- `VOSK_MODEL_PATH`（必須、Vosk 日本語モデルのパス）
- `RTC_ICE_HOST_IPS`（任意、Docker で WebRTC を使う場合にホストIPを指定。カンマ区切り）
- `RTC_ICE_PORT_MIN` / `RTC_ICE_PORT_MAX`（任意、Docker で WebRTC を使う場合の UDP ポート範囲）
- `WS_ADDR`（任意、デフォルト `:8081`。ブラウザとサーバーの音声 WS 用）
- SwitchBot を使う場合: `SWITCHBOT_TOKEN` / `SWITCHBOT_SECRET` / `SWITCHBOT_DEVICE_MAP`

## サーバー（Go）起動
```sh
go run ./cmd/smart-speaker
```
デフォルトで `WS_ADDR=:8081` で `/ws/audio` と `/ws/chat` を開きます。OpenAI からはテキストのみ受信し、ElevenLabs TTS（stream-input）で音声生成→ `audio.play` で返送します。`ELEVENLABS_API_KEY` / `ELEVENLABS_VOICE_ID` 未設定の場合は起動時にエラーになります。

### Docker（Apple Silicon / arm64）
サーバー全体を Docker で実行します。
```sh
docker build -t smart-speaker-server .
docker run --rm -p 8081:8081 -p 50000-50100:50000-50100/udp \
  -e OPENAI_API_KEY=... \
  -e ELEVENLABS_API_KEY=... \
  -e ELEVENLABS_VOICE_ID=... \
  -e ELEVENLABS_MODEL_ID=... \
  -e RTC_ICE_HOST_IPS=... \
  -e RTC_ICE_PORT_MIN=50000 \
  -e RTC_ICE_PORT_MAX=50100 \
  -e SWITCHBOT_TOKEN=... \
  -e SWITCHBOT_SECRET=... \
  -e SWITCHBOT_DEVICE_MAP=... \
  smart-speaker-server
```
Vosk のバイナリ/モデルはイメージ内に含まれ、`VOSK_MODEL_PATH` も設定済みです。

Docker で WebRTC を使う場合、`RTC_ICE_HOST_IPS` にホストの IP を指定してください。
（例: `RTC_ICE_HOST_IPS=192.168.1.10`）
UDP のポート範囲も公開する必要があります。

### Docker Compose
`.env` に環境変数を設定した上で起動します。
```sh
docker compose up --build
```

### 依存ライブラリ
- Vosk の Go バインディングは libvosk を利用します。Docker で完結させるためホストへのインストールは不要です  
  https://github.com/alphacep/vosk-api/blob/master/go/example/README.md
- Opus エンコード/デコードに libopus / libopusfile を利用します  
  https://github.com/hraban/opus

### Vosk モデル（日本語）
- `vosk-model-small-ja-0.22` / `vosk-model-ja-0.22` が利用できます  
  https://alphacephei.com/vosk/models

## フロント（Web）開発
初回のみ依存インストール:
```sh
npm install
```
開発サーバー起動（Vite, ポート5173）:
```sh
npm run dev
```
ブラウザで `http://localhost:5173/` を開いて接続します。  
ブラウザは WebRTC でマイク音声を送信し、サーバー側の Vosk で文字起こしします。TTS 音声は WebRTC で受信して再生します。

## WebSocket プロトコル
- エンドポイント: `ws://<WS_ADDR>/ws/audio` （デフォルト `ws://localhost:8081/ws/audio`）
- 受信（サーバー→ブラウザ）: `{"type":"audio.play","audio":"<base64 pcm16>","role":"assistant"}` を再生
  - WebRTC 移行後は利用しません（移行期間中のみ）

## 構成図（ステージ接続）
- `wschat (/ws/chat)` → `responsesapi` → `tts(ElevenLabs)` → `ws_output (/ws/audio)`
- `toolcaller` ↔ `responsesapi` → `wschat (/ws/chat)` も通知
- `printer` は `responsesapi` のログ出力用（UIには流さない）
 - `rtc` が WebRTC 音声入出力と Vosk 文字起こしを担当

### チャット用 WebSocket
- エンドポイント: `ws://<WS_ADDR>/ws/chat`
- 配信内容（例）:
  - 人間/AI: `{"type":"message","role":"user|assistant|system","text":"...","response_id":"...","final":false}`
  - Function Call: `{"type":"function_call","tool_call_id":"...","name":"...","arguments":{...}}`
  - Function Result: `{"type":"function_result","tool_call_id":"...","output":{...}}`
  - WebRTC offer/answer/ice: `{"type":"webrtc.offer|webrtc.answer|webrtc.ice","sdp":"...","candidate":{...}}`

## 備考
- 旧 PortAudio ベースのマイク/再生は WS 入出力に置き換え済みです
- ハウリング対策は getUserMedia の `echoCancellation` と WebRTC の AEC を利用します  
  https://developer.mozilla.org/en-US/docs/Web/API/MediaTrackConstraints/echoCancellation
