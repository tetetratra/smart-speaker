export type SpeechCallbacks = {
  onFinal: (text: string) => void
  onInterim: (text: string) => void
  onResult?: () => void
  onSpeechEnd?: () => void
  onSoundEnd?: () => void
  onStart?: () => void
  onEnd?: () => void
  onError?: (message: string) => void
}

export type SpeechHandle = {
  isSupported: boolean
  start: (track?: MediaStreamTrack) => void
  stop: () => void
  abort: () => void
}

export function createSpeechRecognizer(callbacks: SpeechCallbacks): SpeechHandle {
  const SpeechRecognition =
    (window as any).SpeechRecognition || (window as any).webkitSpeechRecognition

  if (!SpeechRecognition) {
    return {
      isSupported: false,
      start: () => {},
      stop: () => {},
      abort: () => {},
    }
  }

  const recognition = new SpeechRecognition()
  recognition.lang = 'ja-JP'
  recognition.continuous = true
  recognition.interimResults = true

  let desired = false
  let running = false
  let currentTrack: MediaStreamTrack | undefined

  const startRecognition = () => {
    if (running) return
    try {
      if (currentTrack) {
        ;(recognition as any).start(currentTrack)
      } else {
        recognition.start()
      }
    } catch (err) {
      const name = err instanceof DOMException ? err.name : ''
      if (name === 'InvalidStateError') {
        return
      }
      const message = err instanceof Error ? err.message : 'start error'
      callbacks.onError?.(message)
    }
  }

  recognition.onstart = () => {
    running = true
    callbacks.onStart?.()
  }
  recognition.onend = () => {
    running = false
    callbacks.onEnd?.()
    if (!desired) return
    setTimeout(() => {
      startRecognition()
    }, 200)
  }
  recognition.onspeechend = () => callbacks.onSpeechEnd?.()
  recognition.onsoundend = () => callbacks.onSoundEnd?.()
  recognition.onerror = (event: any) => {
    const message = typeof event?.error === 'string' ? event.error : 'unknown error'
    callbacks.onError?.(message)
  }
  recognition.onresult = (event: any) => {
    callbacks.onResult?.()
    let interim = ''
    const finals: string[] = []
    for (let i = event.resultIndex; i < event.results.length; i += 1) {
      const result = event.results[i]
      const transcript = result?.[0]?.transcript ?? ''
      if (!transcript) continue
      if (result.isFinal) {
        finals.push(transcript)
      } else {
        interim += transcript
      }
    }
    if (interim.trim()) {
      callbacks.onInterim(interim.trim())
    }
    const finalText = finals.join(' ').trim()
    if (finalText) {
      callbacks.onFinal(finalText)
    }
  }

  return {
    isSupported: true,
    start: (track?: MediaStreamTrack) => {
      desired = true
      if (track) {
        currentTrack = track
      }
      if (running) return
      startRecognition()
    },
    stop: () => {
      desired = false
      if (running) {
        recognition.stop()
      }
    },
    abort: () => {
      desired = false
      recognition.abort()
    },
  }
}
