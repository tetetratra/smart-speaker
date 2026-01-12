import hark from 'hark'

export type VADOptions = {
  onStart: () => void
  onStop: () => void
  onVolume?: (db: number) => void
  threshold?: number
  interval?: number
}

export type VADHandle = {
  stop: () => void
}

export function startVAD(stream: MediaStream, options: VADOptions): VADHandle {
  const vad = hark(stream, {
    interval: options.interval ?? 50,
    threshold: options.threshold ?? -50,
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
