export type SpeechCallbacks = {
  onFinal: (text: string) => void
  onInterim: (text: string) => void
  onStart?: () => void
  onEnd?: () => void
  onError?: (message: string) => void
}

export type SpeechHandle = {
  isSupported: boolean
  start: () => void
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

  recognition.onstart = () => callbacks.onStart?.()
  recognition.onend = () => callbacks.onEnd?.()
  recognition.onerror = (event: any) => {
    const message = typeof event?.error === 'string' ? event.error : 'unknown error'
    callbacks.onError?.(message)
  }
  recognition.onresult = (event: any) => {
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
    start: () => recognition.start(),
    stop: () => recognition.stop(),
    abort: () => recognition.abort(),
  }
}
