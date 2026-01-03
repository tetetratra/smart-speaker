type LanguageModelSession = {
  prompt: (parts: any[], options?: { signal?: AbortSignal }) => Promise<string>
  destroy?: () => void
}

type VisionResult = {
  changed: 'yes' | 'no'
  summary: string
  personPresent: 'yes' | 'no'
  activity: string
  raw: string
  timestamp: string
}

type VisionStatus = 'idle' | 'starting' | 'ready' | 'running' | 'unsupported' | 'error'

type VisionOptions = {
  video: HTMLVideoElement
  intervalMs: number
  onResult: (result: VisionResult) => void
  onStatus: (status: VisionStatus, message?: string) => void
}

const buildPrompt = () => {
  return [
    'あなたは2枚の画像の変化判定器兼サマライザです。',
    '1枚目は10秒前、2枚目は現在の画像です。',
    '以下の変化がある場合のみ changed: yes としてください。',
    '- 人間の場所移動',
    '- 人の増減',
    '- 人間の行っている作業の大きな変化',
    'それ以外の変化は無視し changed: no としてください。',
    '出力形式は必ず次の4行にしてください。',
    '',
    'changed: <変化の有無を yes/no で出力>',
    'change: <変化内容の短い説明。変化がない場合は none>',
    'person: <現在の人間の有無を yes/no で出力>',
    'activity: <現在の人間の作業。人がいない場合は none>',
    '',
    '出力例:',
    'changed: yes',
    'change: 机の前に新しく人が座り、ノートPC操作を開始した',
    'person: yes',
    'activity: ノートPCを操作している',
  ].join('\n')
}

async function createSession(onStatus: VisionOptions['onStatus']): Promise<LanguageModelSession> {
  const lm = (window as any).LanguageModel
  if (!lm) {
    onStatus('unsupported', 'LanguageModel APIが見つかりません')
    throw new Error('LanguageModel API is not available')
  }
  onStatus('starting', 'LanguageModel APIを初期化中')
  const session = await lm.create({
    monitor(m: any) {
      if (!m || typeof m.addEventListener !== 'function') return
      m.addEventListener('downloadprogress', (e: any) => {
        if (!e || typeof e.loaded !== 'number') return
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

async function runPrompt(session: LanguageModelSession, previousImage: File, currentImage: File): Promise<string> {
  const promptParts = [
    {
      role: 'user',
      content: [
        { type: 'text', value: buildPrompt() },
        { type: 'image', value: previousImage },
        { type: 'image', value: currentImage },
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

function parseOutput(raw: string): VisionResult {
  const cleaned = raw.trim()
  const changedMatch = cleaned.match(/changed\s*:\s*(yes|no)/i)
  const summaryMatch = cleaned.match(/change\s*:\s*(.*)/i)
  const personMatch = cleaned.match(/person\s*:\s*(yes|no)/i)
  const activityMatch = cleaned.match(/activity\s*:\s*(.*)/i)
  const changed = (changedMatch?.[1]?.toLowerCase() === 'yes' ? 'yes' : 'no') as 'yes' | 'no'
  const summary = summaryMatch?.[1]?.trim() ?? ''
  const personPresent = (personMatch?.[1]?.toLowerCase() === 'yes' ? 'yes' : 'no') as 'yes' | 'no'
  const activity = activityMatch?.[1]?.trim() ?? ''
  return {
    changed,
    summary,
    personPresent,
    activity,
    raw: cleaned,
    timestamp: new Date().toISOString(),
  }
}

export async function startCameraWatcher(options: VisionOptions): Promise<{ stop: () => void }> {
  const { video, intervalMs, onResult, onStatus } = options
  let stopped = false
  let timer: number | undefined
  let previousImage: File | null = null

  onStatus('starting', 'カメラを起動中')
  const stream = await navigator.mediaDevices.getUserMedia({ video: true, audio: false })
  video.srcObject = stream
  await video.play()
  await waitForVideoReady(video)

  let session: LanguageModelSession | null = await createSession(onStatus)
  onStatus('ready', 'カメラ解析を開始')

  const tick = async () => {
    if (stopped) return
    onStatus('running')
    try {
      if (!session) {
        session = await createSession(onStatus)
        onStatus('ready', 'セッションを再初期化しました')
      }
      const currentImage = await captureImageFile(video)
      if (!previousImage) {
        previousImage = currentImage
        onStatus('ready', '初回画像を保存しました')
        return
      }
      const raw = await runPrompt(session, previousImage, currentImage)
      const result = parseOutput(raw)
      previousImage = currentImage
      onResult(result)
    } catch (err) {
      session?.destroy?.()
      session = null
      onStatus('error', err instanceof Error ? err.message : 'unknown error')
    } finally {
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
    session.destroy?.()
    onStatus('idle')
  }

  return { stop }
}

export async function downloadLanguageModel(onStatus: (status: VisionStatus, message?: string) => void): Promise<void> {
  const session = await createSession(onStatus)
  session.destroy?.()
}
