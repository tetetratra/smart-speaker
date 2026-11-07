export interface PlaybackSpeech {
  generationId: number
  segmentId: string
  text: string
  // 区間全体の再生時間。吹き出しを再生終了に合わせてスクロールするのに使う。
  durationMs?: number
}

export interface PlaybackProgress {
  generation_id: number
  segment_id: string
  text?: string
  end?: boolean
  duration_ms?: number
}

export interface AudioReceiver {
  attach(stream: MediaStream): void
  volume(gain: number): void
  progress(frame: PlaybackProgress): void
  stop(generation: number): void
  close(): void
}

// 受信したWebRTC音声を、以前と同じWeb Audioの音量調整を通して再生する。
export function createAudioReceiver(
  onPlaying: (playing: boolean) => void = () => {},
  onSpeech: (speech: PlaybackSpeech) => void = () => {},
  onError: (message: string) => void = () => {},
): AudioReceiver {
  let context: AudioContext | null = null
  let source: MediaStreamAudioSourceNode | null = null
  let gain: GainNode | null = null
  let volume = 1
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
  let generation = 0
  let closed = false
  const advance = (next: number) => {
    if (!Number.isSafeInteger(next) || next < generation) return false
    generation = next
    return true
  }
  return {
    attach(stream) {
      if (closed) return
      release()
      try {
        const Context = window.AudioContext || (window as typeof window & { webkitAudioContext?: typeof AudioContext }).webkitAudioContext
        if (!Context) throw new Error('Web Audio APIを利用できません')
        // マイク取得後、受信トラックが届いた時に作る点も以前の再生経路に合わせる。
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
      if (closed || !Number.isFinite(value)) return
      volume = Math.max(0, Math.min(1.5, value))
      if (gain) gain.gain.value = volume
    },
    progress(frame) {
      if (closed || !advance(frame.generation_id)) return
      onPlaying(!frame.end)
      // 送信開始の通知で表示する。実際の再生とは受信バッファの分だけずれる。
      if (!frame.end && frame.text) {
        onSpeech({ generationId: frame.generation_id, segmentId: frame.segment_id, text: frame.text, durationMs: frame.duration_ms })
      }
    },
    stop(next) {
      if (closed || next <= generation || !advance(next)) return
      // 旧音声の送信停止はサーバーが担う。受信済みの短い残りは流し切り、同じトラックを継続する。
      onPlaying(false)
    },
    close() {
      if (closed) return
      closed = true
      release()
      onPlaying(false)
    },
  }
}
