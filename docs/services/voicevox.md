# VOICEVOX service

VOICEVOX は `tts` component から利用できるローカル TTS provider です。Docker Compose では `voicevox` service として起動し、`server` から `http://voicevox:50021` で接続します。

## Compose service

`docker-compose.yml` には次の service を追加しています。

- service 名: `voicevox`
- image: `voicevox/voicevox_engine:cpu-ubuntu20.04-latest`
- platform: `${VOICEVOX_PLATFORM:-linux/amd64}`
- 接続元: `server`
- endpoint: `http://voicevox:50021`

VOICEVOX Engine の CPU image は arm64 環境でそのまま pull できない場合があるため、Compose では既定で `linux/amd64` を指定しています。Linux amd64 ではネイティブに動作し、Apple Silicon Mac では Docker Desktop の amd64 emulation で動作します。

arm64 対応 image へ切り替える場合や、別 platform を明示したい場合は次のように上書きします。

```sh
VOICEVOX_PLATFORM=linux/arm64 docker compose up
```

`server.depends_on` には `voicevox` を追加していますが、これは起動順の補助です。VOICEVOX Engine の readiness 保証や詳細なヘルスチェックは今回の実装範囲には含めていません。

## 設定

VOICEVOX を使う場合は、`server` に渡す環境変数を次のように設定します。

```sh
TTS_PROVIDER=voicevox
VOICEVOX_ENDPOINT=http://voicevox:50021
VOICEVOX_SPEAKER_ID=1
VOICEVOX_SPEED_SCALE=1.1
```

- `TTS_PROVIDER`: `voicevox` を指定すると VOICEVOX provider を使います。未指定時は `elevenlabs` です。
- `VOICEVOX_ENDPOINT`: VOICEVOX Engine の HTTP endpoint です。Compose 内では `http://voicevox:50021` を使います。
- `VOICEVOX_SPEAKER_ID`: VOICEVOX の speaker ID です。未指定時は `1` です。
- `VOICEVOX_SPEED_SCALE`: 任意です。指定した場合は `/audio_query` の `speedScale` を上書きします。未指定時は engine が返した query をそのまま使います。

`TTS_PROVIDER=voicevox` の場合、ElevenLabs の API key / voice id は TTS stage 初期化には不要です。

## 動作

`tts` component は VOICEVOX Engine に対して次の順で HTTP request を送ります。

1. `POST /audio_query?text=...&speaker=...`
2. `POST /synthesis?speaker=...`

`/synthesis` の response は WAV として読み取り、24kHz / 16bit / mono PCM の `data` chunk だけを `PlayableSpeech.Audio` に流します。WAV が PCM 以外、24kHz 以外、16bit 以外、mono 以外、または `data` chunk 不在の場合はエラーにします。

初回実装ではリサンプリング、ElevenLabs への自動フォールバック、VOICEVOX の readiness check は行いません。

## ログ確認

Compose で起動している場合は、VOICEVOX Engine 側のログを次で確認します。

```sh
docker compose logs voicevox
```

server 側では合成に失敗した speech は下流へ流さず、provider 名付きで synthesize error をログに出します。
