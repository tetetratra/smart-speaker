type LanguageModelSession = {
  prompt: (parts: any[], options?: { signal?: AbortSignal }) => Promise<string>
  destroy?: () => void
}

type PresenceResult = {
  present: 'yes' | 'no'
  raw: string
  capturedAt: string
}

type PresenceStatus = 'idle' | 'starting' | 'ready' | 'running' | 'unsupported' | 'error'

type PresenceOptions = {
  video: HTMLVideoElement
  intervalMs: number
  onResult: (result: PresenceResult) => void
  onStatus: (status: PresenceStatus, message?: string) => void
}

type SessionOptions = {
  reportInit: boolean
  reportDownload: boolean
}

const buildPrompt = () => {
  return [
    'あなたは画像内に人がいるかどうかだけを判定します。',
    '画像内に**確実に**人がいる場合のみ "yes" と答えてください。',
    'いない場合や判定できない場合や、確信がもてない場合は "no" と答えてください。',
    '出力形式は必ず次の1行のみです。',
    '',
    'person: <yes または no>',
    '',
    '出力例:',
    'person: no',
  ].join('\n')
}

async function createSession(
  onStatus: PresenceOptions['onStatus'],
  options: SessionOptions,
): Promise<LanguageModelSession> {
  const lm = (window as any).LanguageModel
  if (!lm) {
    onStatus('unsupported', 'LanguageModel APIが見つかりません')
    throw new Error('LanguageModel API is not available')
  }
  if (options.reportInit) {
    onStatus('starting', 'LanguageModel APIを初期化中')
  }
  const session = await lm.create({
    monitor(m: any) {
      if (!options.reportDownload) return
      if (!m || typeof m.addEventListener !== 'function') return
      m.addEventListener('downloadprogress', (e: any) => {
        if (!e || typeof e.loaded !== 'number') return
        if (e.loaded >= 1) return
        onStatus('starting', `モデルをダウンロード中 (${Math.round(e.loaded * 100)}%)`)
      })
    },
    expectedInputs: [
      { type: 'text', languages: ['ja'] },
      { type: 'image' },
    ],
    expectedOutputs: [{ type: 'text', languages: ['ja'] }],
  })
  return session as LanguageModelSession
}

async function waitForVideoReady(video: HTMLVideoElement): Promise<void> {
  if (video.readyState >= 2 && video.videoWidth > 0 && video.videoHeight > 0) {
    return
  }
  await new Promise<void>((resolve) => {
    const onReady = () => {
      video.removeEventListener('loadedmetadata', onReady)
      resolve()
    }
    video.addEventListener('loadedmetadata', onReady)
  })
}

async function captureImageFile(video: HTMLVideoElement): Promise<File> {
  const canvas = document.createElement('canvas')
  canvas.width = video.videoWidth
  canvas.height = video.videoHeight
  const ctx = canvas.getContext('2d')
  if (!ctx) {
    throw new Error('Canvas context is not available')
  }
  ctx.save()
  ctx.scale(-1, 1)
  ctx.drawImage(video, -canvas.width, 0, canvas.width, canvas.height)
  ctx.restore()
  const blob = await new Promise<Blob>((resolve) => {
    canvas.toBlob((b) => resolve(b as Blob), 'image/jpeg')
  })
  if (!blob) {
    throw new Error('Failed to capture image blob')
  }
  return new File([blob], 'camera.jpeg', { type: 'image/jpeg' })
}

async function runPrompt(session: LanguageModelSession, image: File): Promise<string> {
  const promptParts = [
    {
      role: 'user',
      content: [
        { type: 'text', value: buildPrompt() },
        { type: 'image', value: image },
      ],
    },
  ]
  const controller = new AbortController()
  const timeoutId = window.setTimeout(() => controller.abort(), 20000)
  try {
    return await session.prompt(promptParts, { signal: controller.signal })
  } finally {
    window.clearTimeout(timeoutId)
  }
}

function parseOutput(raw: string): PresenceResult {
  const cleaned = raw.trim()
  const personMatch = cleaned.match(/person\s*:\s*(yes|no)/i)
  const present = (personMatch?.[1]?.toLowerCase() === 'yes' ? 'yes' : 'no') as 'yes' | 'no'
  return {
    present,
    raw: cleaned,
    capturedAt: new Date().toISOString(),
  }
}

export async function startPresenceWatcher(options: PresenceOptions): Promise<{ stop: () => void }> {
  const { video, intervalMs, onResult, onStatus } = options
  let stopped = false
  let timer: number | undefined

  onStatus('starting', 'カメラを起動中')
  const stream = await navigator.mediaDevices.getUserMedia({ video: true, audio: false })
  video.srcObject = stream
  await video.play()
  await waitForVideoReady(video)

  onStatus('ready', 'カメラ解析を開始')

  const tick = async () => {
    if (stopped) return
    onStatus('running')
    let session: LanguageModelSession | null = null
    try {
      session = await createSession(onStatus, { reportInit: false, reportDownload: false })
      const capturedAt = new Date().toISOString()
      const image = await captureImageFile(video)
      const raw = await runPrompt(session, image)
      const result = parseOutput(raw)
      result.capturedAt = capturedAt
      onResult(result)
    } catch (err) {
      onStatus('error', err instanceof Error ? err.message : 'unknown error')
    } finally {
      session?.destroy?.()
      if (!stopped) {
        timer = window.setTimeout(tick, intervalMs)
      }
    }
  }

  timer = window.setTimeout(tick, intervalMs)

  const stop = () => {
    stopped = true
    if (timer) {
      window.clearTimeout(timer)
    }
    stream.getTracks().forEach((track) => track.stop())
    onStatus('idle')
  }

  return { stop }
}

export async function downloadLanguageModel(onStatus: (status: PresenceStatus, message?: string) => void): Promise<void> {
  const session = await createSession(onStatus, { reportInit: true, reportDownload: true })
  session.destroy?.()
}
