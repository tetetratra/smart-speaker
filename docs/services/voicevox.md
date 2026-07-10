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

## `VOICEVOX_SPEAKER_ID` 一覧

`VOICEVOX_SPEAKER_ID` はキャラクター単位の ID ではなく、VOICEVOX Engine の `/speakers` が返す各スタイルの `id` を指定します。つまり、キャラクターと声色をまとめた style ID を 1 つ指定する仕様です。

以下の一覧は、2026-06-04 時点で `voicevox/voicevox_engine:cpu-ubuntu20.04-latest` を起動し、`/speakers` の応答を確認して作成したものです。`latest` tag を参照しているため、将来の VOICEVOX Engine 更新で値やキャラクター数が変わる可能性があります。

| キャラクター | スタイル | `VOICEVOX_SPEAKER_ID` |
| --- | --- | --- |
| 四国めたん | あまあま | `0` |
| ずんだもん | あまあま | `1` |
| 四国めたん | ノーマル | `2` |
| ずんだもん | ノーマル | `3` |
| 四国めたん | セクシー | `4` |
| ずんだもん | セクシー | `5` |
| 四国めたん | ツンツン | `6` |
| ずんだもん | ツンツン | `7` |
| 春日部つむぎ | ノーマル | `8` |
| 波音リツ | ノーマル | `9` |
| 雨晴はう | ノーマル | `10` |
| 玄野武宏 | ノーマル | `11` |
| 白上虎太郎 | ふつう | `12` |
| 青山龍星 | ノーマル | `13` |
| 冥鳴ひまり | ノーマル | `14` |
| 九州そら | あまあま | `15` |
| 九州そら | ノーマル | `16` |
| 九州そら | セクシー | `17` |
| 九州そら | ツンツン | `18` |
| 九州そら | ささやき | `19` |
| もち子さん | ノーマル | `20` |
| 剣崎雌雄 | ノーマル | `21` |
| ずんだもん | ささやき | `22` |
| WhiteCUL | ノーマル | `23` |
| WhiteCUL | たのしい | `24` |
| WhiteCUL | かなしい | `25` |
| WhiteCUL | びえーん | `26` |
| 後鬼 | 人間ver. | `27` |
| 後鬼 | ぬいぐるみver. | `28` |
| No.7 | ノーマル | `29` |
| No.7 | アナウンス | `30` |
| No.7 | 読み聞かせ | `31` |
| 白上虎太郎 | わーい | `32` |
| 白上虎太郎 | びくびく | `33` |
| 白上虎太郎 | おこ | `34` |
| 白上虎太郎 | びえーん | `35` |
| 四国めたん | ささやき | `36` |
| 四国めたん | ヒソヒソ | `37` |
| ずんだもん | ヒソヒソ | `38` |
| 玄野武宏 | 喜び | `39` |
| 玄野武宏 | ツンギレ | `40` |
| 玄野武宏 | 悲しみ | `41` |
| ちび式じい | ノーマル | `42` |
| 櫻歌ミコ | ノーマル | `43` |
| 櫻歌ミコ | 第二形態 | `44` |
| 櫻歌ミコ | ロリ | `45` |
| 小夜/SAYO | ノーマル | `46` |
| ナースロボ＿タイプＴ | ノーマル | `47` |
| ナースロボ＿タイプＴ | 楽々 | `48` |
| ナースロボ＿タイプＴ | 恐怖 | `49` |
| ナースロボ＿タイプＴ | 内緒話 | `50` |
| †聖騎士 紅桜† | ノーマル | `51` |
| 雀松朱司 | ノーマル | `52` |
| 麒ヶ島宗麟 | ノーマル | `53` |
| 春歌ナナ | ノーマル | `54` |
| 猫使アル | ノーマル | `55` |
| 猫使アル | おちつき | `56` |
| 猫使アル | うきうき | `57` |
| 猫使ビィ | ノーマル | `58` |
| 猫使ビィ | おちつき | `59` |
| 猫使ビィ | 人見知り | `60` |
| 中国うさぎ | ノーマル | `61` |
| 中国うさぎ | おどろき | `62` |
| 中国うさぎ | こわがり | `63` |
| 中国うさぎ | へろへろ | `64` |
| 波音リツ | クイーン | `65` |
| もち子さん | セクシー／あん子 | `66` |
| 栗田まろん | ノーマル | `67` |
| あいえるたん | ノーマル | `68` |
| 満別花丸 | ノーマル | `69` |
| 満別花丸 | 元気 | `70` |
| 満別花丸 | ささやき | `71` |
| 満別花丸 | ぶりっ子 | `72` |
| 満別花丸 | ボーイ | `73` |
| 琴詠ニア | ノーマル | `74` |
| ずんだもん | ヘロヘロ | `75` |
| ずんだもん | なみだめ | `76` |
| もち子さん | 泣き | `77` |
| もち子さん | 怒り | `78` |
| もち子さん | 喜び | `79` |
| もち子さん | のんびり | `80` |
| 青山龍星 | 熱血 | `81` |
| 青山龍星 | 不機嫌 | `82` |
| 青山龍星 | 喜び | `83` |
| 青山龍星 | しっとり | `84` |
| 青山龍星 | かなしみ | `85` |
| 青山龍星 | 囁き | `86` |
| 後鬼 | 人間（怒り）ver. | `87` |
| 後鬼 | 鬼ver. | `88` |
| Voidoll | ノーマル | `89` |
| ぞん子 | ノーマル | `90` |
| ぞん子 | 低血圧 | `91` |
| ぞん子 | 覚醒 | `92` |
| ぞん子 | 実況風 | `93` |
| 中部つるぎ | ノーマル | `94` |
| 中部つるぎ | 怒り | `95` |
| 中部つるぎ | ヒソヒソ | `96` |
| 中部つるぎ | おどおど | `97` |
| 中部つるぎ | 絶望と敗北 | `98` |

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
