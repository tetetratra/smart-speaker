# Smart Speaker (Go) + WebSocket 音声 I/O

## 前提
- Go 1.24 以降
- Node 20 以降（フロント開発用）  
  ※ `npm install` でローカルに Vite が入ります（グローバルインストール不要）

## 環境変数
- `OPENAI_API_KEY`（必須）
- `OPENAI_VOICE`（音声モードを使う場合のみ）
- `ELEVENLABS_API_KEY` / `ELEVENLABS_VOICE_ID`（テキスト→音声を ElevenLabs で生成するために必須）
- `ELEVENLABS_MODEL_ID`（任意、デフォルト `eleven_multilingual_v2`）
- `WS_ADDR`（任意、デフォルト `:8081`。ブラウザとサーバーの音声 WS 用）
- SwitchBot を使う場合: `SWITCHBOT_TOKEN` / `SWITCHBOT_SECRET` / `SWITCHBOT_DEVICE_MAP`

## サーバー（Go）起動
```sh
go run ./cmd/smart-speaker
```
デフォルトで `WS_ADDR=:8081` で `/ws/audio` を開きます。OpenAI からはテキストのみ受信し、ElevenLabs TTS（stream-input）で音声生成→ `audio.play` で返送します。`ELEVENLABS_API_KEY` / `ELEVENLABS_VOICE_ID` 未設定の場合は起動時にエラーになります。

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
ブラウザは Web Audio + AudioWorklet でマイクを取得（`echoCancellation: true`）し、PCM16(24kHz) を WS `audio.append` で送信します。受信した `audio.play`（24kHz PCM16）を再生します。

## WebSocket プロトコル
- エンドポイント: `ws://<WS_ADDR>/ws/audio` （デフォルト `ws://localhost:8081/ws/audio`）
- 送信（ブラウザ→サーバー）: `{"type":"audio.append","audio":"<base64 pcm16>"}`
  チャンクは約300ms、24kHz/mono/PCM16 を想定
- 受信（サーバー→ブラウザ）: `{"type":"audio.play","audio":"<base64 pcm16>","role":"assistant"}` を再生

## 構成図（ステージ接続）

```mermaid
flowchart LR
  subgraph Browser
    mic[Mic AudioWorklet\nPCM16] --> wsA[WS /ws/audio]
    wsA --> spk[Audio play]
    wsChat[WS /ws/chat] --> ui[React UI\nチャット表示]
  end

  subgraph Go Server
    wsIn[ws_input\n/ws/audio] --> rt[realtimeapi\nOpenAI Realtime]
    text[textinput] --> rt
    starter[conversationstarter\nsystem prompt] --> rt
    starter --> chat[ws_chat\n/ws/chat]
    rt --> printer[printer\nログのみ]
    rt <-->\n tool[toolcaller]
    rt --> tts[TTS\nElevenLabs stream-input]
    tts --> wsOut[ws_output\n/ws/audio]
    rt --> chat
    tool --> chat
  end

  wsOut --> wsA
  wsChat <-- chat
```

### チャット用 WebSocket
- エンドポイント: `ws://<WS_ADDR>/ws/chat`
- 配信内容（例）:
  - 人間/AI/System: `{"type":"message","role":"user|assistant|system","text":"...","response_id":"...","final":false}`
  - Function Call: `{"type":"function_call","tool_call_id":"...","name":"...","arguments":{...}}`
  - Function Result: `{"type":"function_result","tool_call_id":"...","output":{...}}`
  - conversationstarter 由来の system 文言も `type: "message", role: "system"` として流れます

## 備考
- 旧 PortAudio ベースのマイク/再生は WS 入出力に置き換え済みです
- ハウリング対策は getUserMedia の `echoCancellation` ほかブラウザ/OS の AEC を利用します（マイクの AudioContext.destination への接続はしない実装です）
