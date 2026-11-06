export interface AudioFrame {
  generation_id: number
  segment_id: string
  sequence: number
  audio: string
  sample_rate: number
  final: boolean
  end?: boolean
  text?: string
}

export interface PlaybackSpeech {
  generationId: number
  segmentId: string
  text: string
}

// キューが空でも世代を保持し、後着の旧音声や停止通知で巻き戻さない。
export class GenerationQueue<T extends { generation_id: number }> {
  generation = 0
  private items: T[] = []
  advance(generation: number): boolean {
    if (!Number.isSafeInteger(generation) || generation <= this.generation) return false
    this.generation = generation
    this.items = []
    return true
  }
  push(item: T): boolean {
    if (!Number.isSafeInteger(item.generation_id) || item.generation_id < this.generation) return false
    this.advance(item.generation_id)
    this.items.push(item)
    return true
  }
  shift(): T | undefined { return this.items.shift() }
}

export interface AudioReceiver {
  play(frame: AudioFrame): void
  stop(generation: number): void
  volume(gain: number): void
  close(): void
}

// 先の音声を大量にAudioContextへ予約せず、一件ずつ再生する。
export function createAudioReceiver(
  onPlaying: (playing: boolean) => void = () => {},
  onSpeech: (speech: PlaybackSpeech) => void = () => {},
): AudioReceiver {
  const ctx = new AudioContext()
  const gain = ctx.createGain()
  gain.connect(ctx.destination)
  const queue = new GenerationQueue<AudioFrame>()
  const sources = new Set<AudioBufferSourceNode>()
  const speechTimers = new Set<number>()
  let playhead = ctx.currentTime
  let closed = false
  let ending = false
  const stopSources = () => {
    for (const timer of speechTimers) window.clearTimeout(timer)
    speechTimers.clear()
    for (const source of sources) {
      source.onended = null
      source.stop()
      source.disconnect()
    }
    sources.clear()
    playhead = ctx.currentTime
    ending = false
    onPlaying(false)
  }
  // ごく短い先読みだけを再生エンジンへ予約し、残りは世代付きキューに置く。
  const drain = () => {
    if (closed || ctx.state === 'suspended') return
    let frame: AudioFrame | undefined
    while (playhead < ctx.currentTime + 0.1 && (frame = queue.shift())) {
      if (frame.generation_id !== queue.generation) continue
      if (frame.end) {
        ending = true
        if (!sources.size) onPlaying(false)
        continue
      }
      try {
        const raw = atob(frame.audio)
        if (raw.length === 0 || raw.length % 2 || frame.sample_rate !== 24000) continue
        const buffer = ctx.createBuffer(1, raw.length / 2, frame.sample_rate)
        const pcm = buffer.getChannelData(0)
        for (let i = 0; i < pcm.length; i++) {
          let sample = raw.charCodeAt(i * 2) | raw.charCodeAt(i * 2 + 1) << 8
          if (sample & 0x8000) sample -= 0x10000
          pcm[i] = sample / 32768
        }
        const source = ctx.createBufferSource()
        source.buffer = buffer
        source.connect(gain)
        sources.add(source)
        source.onended = () => {
          source.disconnect()
          sources.delete(source)
          if (ending && !sources.size) onPlaying(false)
          drain()
        }
        const startAt = Math.max(ctx.currentTime + 0.02, playhead)
        playhead = startAt + buffer.duration
        ending = false
        onPlaying(true)
        source.start(startAt)
        if (frame.sequence === 0 && frame.text) {
          const speech: PlaybackSpeech = { generationId: frame.generation_id, segmentId: frame.segment_id, text: frame.text }
          // 受信や生成の完了ではなく、この区間の再生開始に合わせて表示する。
          const timer = window.setTimeout(() => {
            speechTimers.delete(timer)
            if (!closed && speech.generationId === queue.generation) onSpeech(speech)
          }, Math.max(0, (startAt - ctx.currentTime) * 1000))
          speechTimers.add(timer)
        }
      } catch (error) { console.warn('音声データを再生できませんでした', error) }
    }
  }
  const timer = window.setInterval(drain, 10)
  void ctx.resume().catch(() => {})
  return {
    play(frame) {
      if (closed) return
      if (frame.generation_id > queue.generation) stopSources()
      if (!queue.push(frame)) return
      drain()
    },
    stop(generation) {
      if (queue.advance(generation)) stopSources()
    },
    volume(value) { gain.gain.value = value },
    close() {
      closed = true
      window.clearInterval(timer)
      stopSources()
      gain.disconnect()
      void ctx.close()
    },
  }
}
