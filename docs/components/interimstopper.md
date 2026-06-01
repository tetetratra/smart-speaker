# interimstopper component

> [!CAUTION]
> この component は STT が interim transcript（`isFinal=false`）を返すことを前提にしているが、現在の STT モデル `chirp_3`（`internal/components/stt/stage.go`）は、`InterimResults: true` を指定してもストリーミングで interim を返さず final transcript（`isFinal=true`）しか返さない（Chirp 3 の仕様）。
> そのため `stt` は `EventHumanInterimUtterance` を発行せず、この component の早期停止（`generation.Store.Next()`）は実質的に発火しない。AI出力の停止は従来どおり final transcript 到達時のままになる。
> interim による早期停止を実際に効かせるには、interim を返すモデルへ変更する必要がある。

`interimstopper` は、STT の interim transcript をユーザー発話として保存せず、AI出力停止の早期シグナルとして扱う component。
final transcript は従来どおり `utterancebuffer` へ渡す。

## 責務

- `EventHumanInterimUtterance` を受け取ったら、同一発話中の初回だけ `generation.Store.Next()` を呼ぶ。
- generation を前進させ、既存の `generationfilter` に古い LLM/TTS/scheduler 出力を落とさせる。
- interim event は下流へ流さず、会話履歴、LLM入力、UI表示に混ぜない。
- `EventHumanUtterance` を受け取ったら停止済みフラグを解除し、event をそのまま `utterancebuffer` へ渡す。
- `generation.Store` が未設定の場合でも final transcript は通過させる。

## 主な event

- 入力: `EventHumanInterimUtterance`、`EventHumanUtterance`
- 出力: `EventHumanUtterance`

## 接続

```mermaid
flowchart LR
  STT["stt"] -->|"EventHumanInterimUtterance"| IS["interimstopper"]
  STT -->|"EventHumanUtterance"| IS
  IS -.->|"interimでNext"| GEN[("generation.Store")]
  IS -->|"finalのみ通過"| UB["utterancebuffer"]
```

## 動作

1. AI発話中にユーザーの音声が入り、Google STT が interim transcript を返す。
2. `stt` が `EventHumanInterimUtterance` を発行する。
3. `interimstopper` が `generation.Store.Next()` を呼び、現在進行中のAI出力を古いgenerationにする。
4. 同じ発話内で追加のinterimが届いても、finalが届くまではgenerationを追加で進めない。
5. final transcript が届いたら、`EventHumanUtterance` として `utterancebuffer` に渡す。
6. `utterancebuffer` は従来どおりfinal transcriptをbufferし、flush時にユーザー発話用の新しいgenerationを採番する。

RTC出力済みバッファはこの component では破棄しない。
現行のfinal時停止と同じく、generation更新後に後続へ流れてくる古いgenerationの出力を `generationfilter` で止める。
