# Smart Speaker (Go) + WebSocket 音声 I/O

## 前提
- Go 1.24 以降
- Node 20 以降（フロント開発用）  
  ※ `npm install` でローカルに Vite が入ります（グローバルインストール不要）

## 環境変数
- `OPENAI_API_KEY`（必須）
- `WS_ADDR`（任意、デフォルト `:8081`。ブラウザとサーバーの音声 WS 用）
- SwitchBot を使う場合: `SWITCHBOT_TOKEN` / `SWITCHBOT_SECRET` / `SWITCHBOT_DEVICE_MAP`

## サーバー（Go）起動
```sh
go run ./cmd/smart-speaker
```
デフォルトで `WS_ADDR=:8081` で `/ws/audio` を開き、ブラウザからの音声送信 `audio.append` を受け取り、`audio.play` を送信します。

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
ブラウザは Web Audio + AudioWorklet でマイクを取得（`echoCancellation: true`）し、PCM16(16kHz) を WS `audio.append` で送信します。受信した `audio.play` を再生します。

## WebSocket プロトコル
- エンドポイント: `ws://<WS_ADDR>/ws/audio` （デフォルト `ws://localhost:8081/ws/audio`）
- 送信（ブラウザ→サーバー）: `{"type":"audio.append","audio":"<base64 pcm16>"}`  
  チャンクは約300ms、16kHz/mono/PCM16 を想定
- 受信（サーバー→ブラウザ）: `{"type":"audio.play","audio":"<base64 pcm16>","role":"assistant"}` を再生

## 備考
- 旧 PortAudio ベースのマイク/再生は WS 入出力に置き換え済みです
- ハウリング対策は getUserMedia の `echoCancellation` ほかブラウザ/OS の AEC を利用します（マイクの AudioContext.destination への接続はしない実装です）
