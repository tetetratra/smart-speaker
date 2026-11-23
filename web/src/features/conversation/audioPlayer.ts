// 受信した WebRTC の音声トラックを、Web Audio の GainNode に通して再生する。音量はここで変える。
// 何を読み上げ中かといった画面の状態は扱わない（sessionReducer が受け持つ）。

export type AudioPlayer = {
  attach(stream: MediaStream): void
  // 0〜1.5 の倍率。
  volume(gain: number): void
  close(): void
}

export function createAudioPlayer(initialVolume: number, onError: (message: string) => void = () => {}): AudioPlayer {
  let context: AudioContext | null = null
  let source: MediaStreamAudioSourceNode | null = null
  let gain: GainNode | null = null
  let volume = clampVolume(initialVolume) ?? 1
  let closed = false
  const release = () => {
    source?.disconnect()
    source = null
    gain?.disconnect()
    gain = null
    if (context) {
      void context.close().catch(() => {})
      context = null
    }
  }
  return {
    attach(stream) {
      if (closed) return
      release()
      try {
        const Context = window.AudioContext || (window as typeof window & { webkitAudioContext?: typeof AudioContext }).webkitAudioContext
        if (!Context) throw new Error('Web Audio APIを利用できません')
        // 受信トラックが届いたとき（マイクの許可を得た後）に作る。ユーザーの操作の後でないと再生を始められない端末があるため。
        const current = new Context()
        context = current
        source = current.createMediaStreamSource(stream)
        gain = current.createGain()
        gain.gain.value = volume
        source.connect(gain)
        gain.connect(current.destination)
        if (current.state === 'suspended') {
          void current.resume().catch(() => {
            if (!closed && context === current) onError('音声を再生できませんでした。接続し直してください。')
          })
        }
      } catch {
        release()
        onError('音声を再生できませんでした。接続し直してください。')
      }
    },
    volume(value) {
      const next = clampVolume(value)
      if (closed || next === undefined) return
      volume = next
      if (gain) gain.gain.value = volume
    },
    close() {
      if (closed) return
      closed = true
      release()
    },
  }
}

function clampVolume(value: number): number | undefined {
  if (!Number.isFinite(value)) return undefined
  return Math.max(0, Math.min(1.5, value))
}
