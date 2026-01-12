import hark from 'hark'

export type VADOptions = {
  onStart: () => void
  onStop: () => void
  onVolume?: (db: number) => void
  threshold?: number
  interval?: number
  history?: number
}

export type VADHandle = {
  stop: () => void
}

export function startVAD(stream: MediaStream, options: VADOptions): VADHandle {
  const vad = hark(stream, {
    interval: options.interval ?? 150, // 音量判定のサンプリング周期(ms)
    threshold: options.threshold ?? -50, // 発話判定のしきい値(dB)
    history: options.history ?? 10, // 無音判定の履歴数(回)
  })

  vad.on('speaking', options.onStart)
  vad.on('stopped_speaking', options.onStop)
  if (options.onVolume) {
    vad.on('volume_change', (db: number) => options.onVolume?.(db))
  }

  return {
    stop: () => {
      vad.stop()
      stream.getTracks().forEach((t) => t.stop())
    },
  }
}
