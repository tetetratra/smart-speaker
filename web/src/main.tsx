import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import ReactDOM from 'react-dom/client'
import { createWS } from './ws'
import { conversationBubble } from './conversation'
import { Whiteboard } from './Whiteboard'
import { WakeModeControl } from './WakeModeControl'
import { AutomationView } from './AutomationView'
import { createAudioReceiver, type AudioReceiver, type PlaybackSpeech } from './audio'

type ChatMessage =
  | { id: number; type: 'user' | 'agent' | 'tool_call' | 'tool_result' | 'system'; text: string; responseId?: string; final?: boolean; source?: string; generationId?: number }

type BoardEntry = { id: number; content: string; timestamp: number }
type ToolToastKind = 'call' | 'result'
type ToolToast = { id: number; kind: ToolToastKind; toolName: string }
type ServerEventLog = { id: number; line: string }
type UiMode = 'admin' | 'app' | 'memory' | 'scenes' | 'jobs'
type MemoryItem = {
  id: string
  content: string
  tags: string[]
  created_at: string
  updated_at: string
}
type MemoryListResponse = { memories: MemoryItem[] }
type MemoryWriteResponse = { memory: MemoryItem }
type SwitchBotSceneItem = {
  id: string
  name: string
}
type SwitchBotSceneListResponse = { scenes: SwitchBotSceneItem[] }

const browserURL = new URL(window.location.href)
const backendURL = new URL(window.location.origin)
if (browserURL.port === '5173') {
  backendURL.port = '8081'
}
const wsProtocol = backendURL.protocol === 'https:' ? 'wss' : 'ws'
const chatWSUrl = `${wsProtocol}://${backendURL.host}/ws/chat`
const reconnectMaxAttempts = 10
const reconnectInitialDelayMs = 1000
const toolToastDurationMs = 5000
const serverEventLogLimit = 1000

type PlaybackVolumeLevel = 'mute' | 'quiet' | 'low' | 'normal' | 'boost'

const playbackVolumePresets: Record<PlaybackVolumeLevel, { label: string; gain: number }> = {
  mute: { label: '0倍', gain: 0 },
  quiet: { label: '0.3倍', gain: 0.3 },
  low: { label: '0.6倍', gain: 0.6 },
  normal: { label: '1倍', gain: 1.0 },
  boost: { label: '1.5倍', gain: 1.5 },
}
const playbackVolumeLevels: PlaybackVolumeLevel[] = ['mute', 'quiet', 'low', 'normal', 'boost']
const defaultPlaybackVolumeLevel: PlaybackVolumeLevel = 'boost'

const liveRootStyle = `
  :root {
    color-scheme: light;
    --ui-accent: #2563eb;
    --ui-accent-text: #ffffff;
    --ui-accent-soft: #dbeafe;
    --ui-accent-soft-text: #1d4ed8;
    --ui-tool-call-bg: #dcfce7;
    --ui-tool-call-line: #86efac;
    --ui-tool-call-text: #166534;
    --ui-danger-bg: #fef2f2;
    --ui-danger-line: #fecaca;
    --ui-danger-text: #991b1b;
    --live-bg: #f6f6f4;
    --live-panel: #ffffff;
    --live-panel-soft: #fafafa;
    --live-panel-raised: #ffffff;
    --live-text: #1f1f1f;
    --live-muted: #6f6f6f;
    --live-line: #e2e2e2;
    --live-line-strong: #e2e2e2;
    --live-shadow: rgba(0,0,0,0.04);
    --live-inset-line: #f0f0f0;
    --live-toggle-bg: #e9e9e9;
    --live-toggle-on: #2f6fde;
    --live-toggle-knob: #ffffff;
  }
  * { box-sizing: border-box; }
  html, body { height: 100%; overflow: hidden; }
  body {
    margin: 0;
    background: var(--live-bg);
    color: var(--live-text);
    font-family: "IBM Plex Sans", system-ui, sans-serif;
  }
  body.admin-mode {
    overflow: auto;
  }
  .live-frame {
    width: 100vw;
    height: 100vh;
    height: 100dvh;
    padding: 6px;
    position: relative;
    background: var(--live-bg);
    color: var(--live-text);
  }
  @media (prefers-color-scheme: dark) {
    :root {
      color-scheme: dark;
      --ui-accent: #2563eb;
      --ui-accent-text: #ffffff;
      --ui-accent-soft: #1e3558;
      --ui-accent-soft-text: #bfdbfe;
      --ui-tool-call-bg: #143322;
      --ui-tool-call-line: #357a4c;
      --ui-tool-call-text: #bbf7d0;
      --ui-danger-bg: #381c26;
      --ui-danger-line: #864354;
      --ui-danger-text: #fecdd3;
      --live-bg: #050608;
      --live-panel: #101217;
      --live-panel-soft: #171a21;
      --live-panel-raised: #1b1f28;
      --live-text: #f4f7fb;
      --live-muted: #aab2c0;
      --live-line: #2a2f3a;
      --live-line-strong: #3a4250;
      --live-shadow: rgba(0,0,0,0.38);
      --live-inset-line: #202632;
      --live-toggle-bg: #303643;
      --live-toggle-on: #4f8cff;
      --live-toggle-knob: #f4f7fb;
    }
  }
  textarea, input, select {
    background: var(--live-panel);
    color: var(--live-text);
  }
  .live-main {
    width: 100%;
    height: 100%;
    background: var(--live-panel);
    border: 1px solid var(--live-line);
    border-radius: 14px;
    padding: 12px;
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(248px, 40%);
    gap: 12px;
    min-height: 0;
  }
  .live-left {
    min-width: 0;
    min-height: 0;
    display: grid;
    grid-template-rows: 6fr 4fr;
    gap: 12px;
  }
  .live-right {
    min-width: 0;
    min-height: 0;
    display: grid;
    grid-template-rows: auto 1fr auto;
    gap: 8px;
  }
  .live-controls-row {
    display: flex;
    justify-content: flex-end;
    gap: 6px;
    width: 100%;
    align-items: center;
  }
  .live-controls-actions {
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: 6px;
    align-items: center;
    flex: 1 1 0;
    min-width: 0;
  }
  .live-volume-control {
    border: 1px solid var(--live-line);
    background: var(--live-panel-soft);
    border-radius: 10px;
    padding: 8px 10px 7px;
    position: relative;
    overflow: hidden;
    min-height: 0;
    flex: 1;
  }
  .live-tool-toast-stack {
    position: absolute;
    top: 8px;
    left: 10px;
    right: 10px;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .live-tool-toast {
    min-height: 32px;
    border: 1px solid transparent;
    border-radius: 8px;
    padding: 7px 9px;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    font-size: 12px;
    line-height: 1.2;
    font-weight: 700;
    box-shadow: 0 4px 12px rgba(0,0,0,0.08);
    transform-origin: top center;
    will-change: transform, opacity;
    pointer-events: none;
    animation: live-tool-toast-slide 5000ms cubic-bezier(0.22, 1, 0.36, 1) forwards;
  }
  .live-tool-toast.call {
    background: var(--ui-tool-call-bg);
    border-color: var(--ui-tool-call-line);
    color: var(--ui-tool-call-text);
  }
  .live-tool-toast.result {
    background: var(--ui-accent-soft);
    border-color: var(--live-toggle-on);
    color: var(--ui-accent-soft-text);
  }
  .live-tool-toast-tool {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  @keyframes live-tool-toast-slide {
    0% {
      opacity: 0;
      transform: translateY(-10px) scale(0.98);
    }
    18% {
      opacity: 1;
      transform: translateY(1px) scale(1);
    }
    28% {
      opacity: 1;
      transform: translateY(0) scale(1);
    }
    76% {
      opacity: 1;
      transform: translateY(0) scale(1);
    }
    100% {
      opacity: 0;
      transform: translateY(-6px) scale(0.985);
    }
  }
  .live-volume-slider {
    width: 100%;
    height: 10px;
    margin: 0;
    cursor: pointer;
    accent-color: var(--live-toggle-on);
    -webkit-appearance: none;
    appearance: none;
    background: transparent;
  }
  .live-volume-slider::-webkit-slider-runnable-track {
    height: 4px;
    border-radius: 999px;
    background: var(--live-toggle-bg);
  }
  .live-volume-slider::-webkit-slider-thumb {
    -webkit-appearance: none;
    appearance: none;
    width: 12px;
    height: 12px;
    margin-top: -4px;
    border: 1px solid var(--live-line);
    border-radius: 50%;
    background: var(--live-toggle-knob);
    box-shadow: 0 1px 2px var(--live-shadow);
  }
  .live-volume-slider::-moz-range-track {
    height: 4px;
    border: none;
    border-radius: 999px;
    background: var(--live-toggle-bg);
  }
  .live-volume-slider::-moz-range-thumb {
    width: 12px;
    height: 12px;
    border: 1px solid var(--live-line);
    border-radius: 50%;
    background: var(--live-toggle-knob);
    box-shadow: 0 1px 2px var(--live-shadow);
  }
  .live-playback-slider-row {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
  }
  .live-playback-slider-label {
    flex-shrink: 0;
    width: 52px;
    font-size: 10px;
    line-height: 1;
    color: var(--live-muted);
    white-space: nowrap;
  }
  .live-playback-slider-row .live-volume-slider {
    flex: 1;
    min-width: 0;
  }
  .live-status-grid {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 4px;
    width: 100%;
  }
  .live-status-card {
    min-width: 0;
    min-height: 40px;
    padding: 4px 3px;
    border: 1px solid var(--live-line);
    border-radius: 12px;
    background: var(--live-panel-soft);
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 2px;
    text-align: center;
  }
  .live-status-label {
    font-size: 10px;
    line-height: 1.1;
    color: var(--live-muted);
  }
  .live-status-value {
    font-size: 11px;
    line-height: 1.1;
    font-weight: 700;
    color: var(--live-text);
    white-space: nowrap;
  }
  .live-control-btn,
  .live-admin-btn {
    border-radius: 999px;
    padding: 4px 8px;
    font-size: 11px;
    font-weight: 700;
    border: 1px solid var(--live-line);
    background: var(--live-panel-soft);
    color: var(--live-muted);
    align-self: center;
    white-space: nowrap;
    min-height: 30px;
    line-height: 1;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 8px;
  }
  .live-control-btn {
    width: auto;
    justify-content: flex-start;
  }
  .live-admin-btn {
    width: auto;
    padding-inline: 10px;
  }
  .live-toggle-switch {
    width: 36px;
    height: 20px;
    border-radius: 999px;
    background: var(--live-toggle-bg);
    border: 1px solid var(--live-line);
    position: relative;
    display: inline-flex;
    align-items: center;
    flex: 0 0 auto;
  }
  .live-toggle-switch::after {
    content: "";
    width: 14px;
    height: 14px;
    border-radius: 999px;
    background: var(--live-toggle-knob);
    border: 1px solid var(--live-line);
    position: absolute;
    left: 2px;
  }
  .live-toggle-switch.on {
    background: var(--live-toggle-on);
    border-color: var(--live-toggle-on);
  }
  .live-toggle-switch.on::after {
    left: 18px;
  }
  .live-bubble-slot {
    min-width: 0;
    min-height: 0;
    display: flex;
    flex-direction: column;
  }
  .live-bubble {
    background: var(--live-panel-raised);
    border: 1px solid var(--live-line);
    border-radius: 14px;
    padding: 12px 14px;
    font-size: 24px;
    line-height: 1.3;
    position: relative;
    box-shadow: 0 1px 2px var(--live-shadow);
    text-align: center;
    display: flex;
    flex: 1;
    flex-direction: column;
    align-items: stretch;
    min-height: 0;
    min-width: 0;
    overflow: visible;
  }
  .live-bubble-content {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 10px;
    width: 100%;
    flex: 1;
    min-height: 0;
    overflow: auto;
    overscroll-behavior: contain;
    -webkit-overflow-scrolling: touch;
  }
  .live-bubble-user {
    color: var(--live-muted);
    font-size: 18px;
    line-height: 1.35;
    opacity: 0.72;
    overflow-wrap: anywhere;
    white-space: pre-wrap;
  }
  .live-bubble-agent {
    overflow-wrap: anywhere;
    white-space: pre-wrap;
  }
  .live-bubble::after {
    content: "";
    position: absolute;
    right: -8px;
    top: 40%;
    width: 14px;
    height: 14px;
    background: var(--live-panel-raised);
    border-right: 1px solid var(--live-line);
    border-top: 1px solid var(--live-line);
    transform: rotate(45deg);
  }
  .live-bubble-ai-speaking {
    border-color: var(--live-toggle-on);
    box-shadow:
      0 0 0 2px color-mix(in srgb, var(--live-toggle-on) 35%, transparent),
      0 0 12px color-mix(in srgb, var(--live-toggle-on) 45%, transparent);
  }
  .live-bubble-ai-speaking::after {
    border-right-color: var(--live-toggle-on);
    border-top-color: var(--live-toggle-on);
  }
  .live-board {
    background: var(--live-panel-soft);
    border: 2px solid var(--live-line-strong);
    border-radius: 10px;
    padding: 10px 12px;
    font-size: 13px;
    line-height: 1.4;
    color: var(--live-text);
    box-shadow: inset 0 0 0 1px var(--live-inset-line);
    overflow: auto;
    min-height: 0;
    height: 100%;
  }
  .live-board-empty {
    color: var(--live-muted);
    opacity: 0.72;
  }
  .live-board-entry {
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  .live-board-separator {
    border: 0;
    border-top: 1px solid var(--live-line);
    margin: 10px 0;
  }
  .live-controls-panel {
    display: grid;
    grid-template-rows: auto auto auto;
    gap: 8px;
    min-width: 0;
    min-height: 0;
  }
  .live-character-area {
    display: flex;
    flex-direction: column;
    min-width: 0;
    min-height: 0;
  }
  @media (orientation: portrait) {
    .live-main {
      grid-template-columns: 1fr;
      grid-template-rows: auto repeat(3, minmax(0, 1fr));
      gap: 8px;
    }
    .live-left,
    .live-right {
      display: contents;
    }
    .live-controls-panel {
      grid-row: 1;
    }
    .live-board {
      grid-row: 2;
    }
    .live-character-area {
      grid-row: 3;
    }
    .live-bubble-slot {
      grid-row: 4;
    }
    .live-bubble {
      width: 100%;
      font-size: clamp(14px, 4vw, 20px);
    }
    .live-bubble-user {
      font-size: clamp(12px, 3.2vw, 16px);
    }
    .live-bubble::after {
      right: auto;
      bottom: auto;
      left: 50%;
      top: -6px;
      width: 11px;
      height: 11px;
      border-right: none;
      border-bottom: none;
      border-left: 1px solid var(--live-line);
      border-top: 1px solid var(--live-line);
      transform: translateX(-50%) rotate(45deg);
    }
    .live-bubble-ai-speaking::after {
      border-right-color: transparent;
      border-bottom-color: transparent;
      border-left-color: var(--live-toggle-on);
      border-top-color: var(--live-toggle-on);
    }
  }
`

function isBinaryLikeValue(value: unknown): boolean {
  return value instanceof ArrayBuffer || ArrayBuffer.isView(value) || value instanceof Blob
}

function isBinaryLikeKey(key: string): boolean {
  return /audio|pcm|sample|prebuffer|bytes|buffer|blob|binary|data/i.test(key)
}

function isBase64Like(value: string): boolean {
  if (value.length < 256 || value.length % 4 !== 0) return false
  return /^[A-Za-z0-9+/]+={0,2}$/.test(value)
}

function sanitizeEventValue(value: unknown, key = ''): unknown {
  if (isBinaryLikeValue(value)) return '...'
  if (typeof value === 'string') {
    if (isBinaryLikeKey(key) || isBase64Like(value)) return '...'
    return value
  }
  if (Array.isArray(value)) {
    if (isBinaryLikeKey(key) && value.length > 0) return '...'
    return value.map((item) => sanitizeEventValue(item))
  }
  if (value && typeof value === 'object') {
    const source = value as Record<string, unknown>
    const result: Record<string, unknown> = {}
    if (Object.prototype.hasOwnProperty.call(source, 'type')) {
      result.type = sanitizeEventValue(source.type, 'type')
    }
    for (const [entryKey, entryValue] of Object.entries(source)) {
      if (entryKey === 'type') continue
      result[entryKey] = sanitizeEventValue(entryValue, entryKey)
    }
    return result
  }
  return value
}

function formatServerEventLog(raw: unknown): string {
  const sanitized = sanitizeEventValue(raw)
  try {
    return JSON.stringify(sanitized)
  } catch {
    return JSON.stringify({ type: 'unserializable', value: '...' })
  }
}

function formatMemoryDate(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat('ja-JP', {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(date)
}

function normalizeMemoryItem(memory: MemoryItem): MemoryItem {
  return {
    id: String(memory.id),
    content: String(memory.content),
    tags: Array.isArray(memory.tags) ? memory.tags.map(String) : [],
    created_at: String(memory.created_at),
    updated_at: String(memory.updated_at),
  }
}

function normalizeSwitchBotSceneItem(scene: SwitchBotSceneItem): SwitchBotSceneItem {
  return {
    id: String(scene.id),
    name: String(scene.name),
  }
}

function isScrolledToBottom(el: HTMLElement): boolean {
  return el.scrollHeight - el.scrollTop - el.clientHeight <= 4
}

type LiveViewProps = {
  connected: boolean
  connecting: boolean
  speechDetectStatus: string
  sttStatus: string
  lastAssistantMessage: string
  lastUserMessage: string
  isAiSpeaking: boolean
  isConversationBubbleHidden: boolean
  boardEntries: BoardEntry[]
  toolToasts: ToolToast[]
  playbackVolumeLevel: PlaybackVolumeLevel
  onPlaybackVolumeChange: (level: PlaybackVolumeLevel) => void
  connect: () => Promise<void>
  disconnect: () => void
  goScenes: () => void
  goMemory: () => void
  goAdmin: () => void
  goJobs: () => void
}

function App() {
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [playbackSpeech, setPlaybackSpeech] = useState<PlaybackSpeech>()
  const [connected, setConnected] = useState(false)
  const [busy, setBusy] = useState(false)
  const [rtcStatus, setRtcStatus] = useState('停止中')
  const [rtcError, setRtcError] = useState('')
  const [audioSendStatus, setAudioSendStatus] = useState('停止中')
  const [speechDetectStatus, setSpeechDetectStatus] = useState('待機中')
  const [sttStatus, setSttStatus] = useState('停止中')
  const [sttError, setSttError] = useState('')
  const [playbackVolumeLevel, setPlaybackVolumeLevel] = useState<PlaybackVolumeLevel>(defaultPlaybackVolumeLevel)
  const [boardEntries, setBoardEntries] = useState<BoardEntry[]>([])
  const [toolToasts, setToolToasts] = useState<ToolToast[]>([])
  const [serverEventLogs, setServerEventLogs] = useState<ServerEventLog[]>([])
  const [memories, setMemories] = useState<MemoryItem[]>([])
  const [memoryLoading, setMemoryLoading] = useState(false)
  const [memoryError, setMemoryError] = useState('')
  const [switchBotScenes, setSwitchBotScenes] = useState<SwitchBotSceneItem[]>([])
  const [switchBotSceneLoading, setSwitchBotSceneLoading] = useState(false)
  const [switchBotSceneError, setSwitchBotSceneError] = useState('')
  const loadMemories = useCallback((signal?: AbortSignal) => {
    setMemoryLoading(true)
    setMemoryError('')

    return fetch(new URL('/api/memories', backendURL).toString(), { signal })
      .then(async (resp) => {
        if (!resp.ok) {
          throw new Error(`メモリ一覧を取得できませんでした (${resp.status})`)
        }
        return await resp.json() as MemoryListResponse
      })
      .then((data) => {
        if (!Array.isArray(data.memories)) {
          throw new Error('メモリ一覧の形式が不正です')
        }
        setMemories(data.memories.map(normalizeMemoryItem))
      })
      .catch((err) => {
        if (signal?.aborted) return
        setMemoryError(err instanceof Error ? err.message : 'メモリ一覧を取得できませんでした')
      })
      .finally(() => {
        if (!signal?.aborted) {
          setMemoryLoading(false)
        }
      })
  }, [])
  const loadSwitchBotScenes = useCallback((signal?: AbortSignal) => {
    setSwitchBotSceneLoading(true)
    setSwitchBotSceneError('')

    return fetch(new URL('/api/switchbot/scenes', backendURL).toString(), { signal })
      .then(async (resp) => {
        if (!resp.ok) {
          throw new Error(`シーン一覧を取得できませんでした (${resp.status})`)
        }
        return await resp.json() as SwitchBotSceneListResponse
      })
      .then((data) => {
        if (!Array.isArray(data.scenes)) {
          throw new Error('シーン一覧の形式が不正です')
        }
        setSwitchBotScenes(data.scenes.map(normalizeSwitchBotSceneItem))
      })
      .catch((err) => {
        if (signal?.aborted) return
        setSwitchBotSceneError(err instanceof Error ? err.message : 'シーン一覧を取得できませんでした')
      })
      .finally(() => {
        if (!signal?.aborted) {
          setSwitchBotSceneLoading(false)
        }
      })
  }, [])
  const [isAiSpeaking, setIsAiSpeaking] = useState(false)
  const currentGenerationRef = useRef(0)
  const idRef = useRef(0)
  const adminLogRef = useRef<HTMLDivElement | null>(null)
  const shouldFollowAdminLogRef = useRef(true)
  const audioReceiverRef = useRef<AudioReceiver | null>(null)

  const wsChatRef = useRef<ReturnType<typeof createWS> | null>(null)
  const openConnectionRef = useRef<((isAutoReconnect: boolean) => Promise<void>) | null>(null)
  const peerRef = useRef<RTCPeerConnection | null>(null)
  const micStreamRef = useRef<MediaStream | null>(null)
  const pendingICERef = useRef<RTCIceCandidateInit[]>([])
  const reconnectAttemptRef = useRef(0)
  const reconnectTimerRef = useRef<number | null>(null)
  const manualDisconnectRef = useRef(false)
  const lastAudioBytesSentRef = useRef<number | null>(null)

  const nextMessageId = useCallback(() => {
    idRef.current += 1
    return idRef.current
  }, [])

  const appendMessage = useCallback((msg: ChatMessage) => {
    setMessages((prev) => [...prev, msg])
  }, [])

  const resetConversationBubble = useCallback(() => {
    setIsAiSpeaking(false)
    setPlaybackSpeech(undefined)
    setMessages([])
  }, [])

  const updateAdminLogFollow = useCallback(() => {
    const el = adminLogRef.current
    shouldFollowAdminLogRef.current = !el || isScrolledToBottom(el)
  }, [])

  const appendServerEventLog = useCallback((raw: unknown) => {
    updateAdminLogFollow()
    setServerEventLogs((prev) => {
      const next = [...prev, { id: nextMessageId(), line: formatServerEventLog(raw) }]
      return next.length > serverEventLogLimit ? next.slice(-serverEventLogLimit) : next
    })
  }, [nextMessageId, updateAdminLogFollow])

  const showToolToast = useCallback((kind: ToolToastKind, toolName: string) => {
    const normalizedToolName = toolName.trim() || 'unknown_tool'
    const id = nextMessageId()
    setToolToasts((prev) => [{ id, kind, toolName: normalizedToolName }, ...prev.slice(0, 1)])
    window.setTimeout(() => {
      setToolToasts((prev) => prev.filter((toast) => toast.id !== id))
    }, toolToastDurationMs)
  }, [nextMessageId])

  const selectedPlaybackVolume = playbackVolumePresets[playbackVolumeLevel]

  const stopRemoteAudioGraph = useCallback(() => {
    audioReceiverRef.current?.close()
    audioReceiverRef.current = null
  }, [])

  useEffect(() => {
    audioReceiverRef.current?.volume(selectedPlaybackVolume.gain)
  }, [selectedPlaybackVolume.gain])

  const handleRTCSignal = useCallback(async (raw: any) => {
    const peer = peerRef.current
    if (!peer) return
    if (raw.type === 'webrtc.answer') {
      if (!raw.sdp) return
      try {
        await peer.setRemoteDescription({ type: 'answer', sdp: String(raw.sdp) })
        const pending = pendingICERef.current
        pendingICERef.current = []
        for (const cand of pending) {
          await peer.addIceCandidate(cand)
        }
      } catch (err) {
        setRtcError(err instanceof Error ? err.message : 'RTC answer error')
      }
      return
    }
    if (raw.type === 'webrtc.ice') {
      if (!raw.candidate) return
      const candidate = raw.candidate as RTCIceCandidateInit
      if (!peer.remoteDescription) {
        pendingICERef.current.push(candidate)
        return
      }
      try {
        await peer.addIceCandidate(candidate)
      } catch (err) {
        setRtcError(err instanceof Error ? err.message : 'RTC ice error')
      }
    }
  }, [])

  const handleChatMessage = useCallback(
    (raw: any) => {
      appendServerEventLog(raw)
      if (!raw || typeof raw !== 'object') return
      const generation = Number.isSafeInteger(raw.generation_id) ? raw.generation_id as number : undefined
      if (generation !== undefined) currentGenerationRef.current = Math.max(currentGenerationRef.current, generation)
      const isCurrentGeneration = generation === undefined || generation === currentGenerationRef.current
      if (raw.type === 'webrtc.answer' || raw.type === 'webrtc.ice') {
        handleRTCSignal(raw)
        return
      }
      switch (raw.type) {
        case 'message': {
          const text = typeof raw.text === 'string' ? raw.text : ''
          if (!text) return
          let role: ChatMessage['type'] = 'agent'
          if (raw.role === 'user') role = 'user'
          else if (raw.role === 'system') role = 'system'
          else if (raw.role === 'agent') role = 'agent'
          else if (raw.role === 'tool_call') role = 'tool_call'
          else if (raw.role === 'tool_result') role = 'tool_result'
          if (role === 'tool_call' || role === 'tool_result') {
            showToolToast(role === 'tool_call' ? 'call' : 'result', typeof raw.source === 'string' ? raw.source : '')
          }
          if (role === 'user' && isCurrentGeneration) {
            setIsAiSpeaking(false)
            setSttStatus('完了')
            setSpeechDetectStatus('待機中')
          }
          const displayText = raw.role ? text : `(roleなし) ${text}`
          appendMessage({
            id: nextMessageId(),
            type: role,
            text: displayText,
            responseId: raw.response_id,
            final: raw.final,
            source: typeof raw.source === 'string' ? raw.source : undefined,
            generationId: generation,
          })
          break
        }
        case 'audio_frame': {
          audioReceiverRef.current?.play(raw)
          break
        }
        case 'playback_stop': {
          audioReceiverRef.current?.stop(raw.generation_id)
          setPlaybackSpeech((speech) => speech && speech.generationId < raw.generation_id ? undefined : speech)
          break
        }
        case 'realtime_status': {
          setSttStatus(raw.status === 'connected' ? '待機中' : raw.status === 'error' ? 'エラー' : '接続中')
          setSttError(raw.message || '')
          break
        }
        case 'speech_start': {
          setIsAiSpeaking(false)
          setSpeechDetectStatus('検出中')
          setSttStatus('認識中')
          break
        }
        case 'speech_end': {
          setSpeechDetectStatus('待機中')
          setSttStatus('最終結果待ち')
          break
        }
        case 'agent_speech_end': {
          setIsAiSpeaking(false)
          break
        }
        case 'session_reset': {
          resetConversationBubble()
          break
        }
        case 'whiteboard_update': {
          const content = typeof raw.content === 'string' ? raw.content.trim() : ''
          if (!content) return
          setBoardEntries((prev) => [...prev, { id: nextMessageId(), content, timestamp: Date.now() }])
          break
        }
        default:
          break
      }
    },
    [appendMessage, appendServerEventLog, handleRTCSignal, nextMessageId, resetConversationBubble, showToolToast],
  )

  const stopRTC = useCallback(() => {
    if (peerRef.current) {
      peerRef.current.onicecandidate = null
      peerRef.current.ontrack = null
      peerRef.current.onconnectionstatechange = null
      peerRef.current.close()
      peerRef.current = null
    }
    if (micStreamRef.current) {
      micStreamRef.current.getTracks().forEach((t) => t.stop())
      micStreamRef.current = null
    }
    stopRemoteAudioGraph()
    pendingICERef.current = []
    lastAudioBytesSentRef.current = null
    setRtcStatus('停止中')
    setAudioSendStatus('停止中')
    setSpeechDetectStatus('待機中')
    setRtcError('')
    setSttStatus('停止中')
  }, [stopRemoteAudioGraph])

  const clearReconnectTimer = useCallback(() => {
    if (reconnectTimerRef.current === null) return
    window.clearTimeout(reconnectTimerRef.current)
    reconnectTimerRef.current = null
  }, [])

  const scheduleReconnect = useCallback(() => {
    if (manualDisconnectRef.current) return
    if (reconnectTimerRef.current !== null) return
    if (reconnectAttemptRef.current >= reconnectMaxAttempts) {
      console.warn(`[ws reconnect] retry limit reached: ${reconnectMaxAttempts}`)
      return
    }
    const attempt = reconnectAttemptRef.current + 1
    reconnectAttemptRef.current = attempt
    const delayMs = reconnectInitialDelayMs * Math.pow(2, attempt-1)
    console.log(`[ws reconnect] attempt ${attempt}/${reconnectMaxAttempts} in ${delayMs}ms`)
    reconnectTimerRef.current = window.setTimeout(() => {
      reconnectTimerRef.current = null
      if (manualDisconnectRef.current) return
      void openConnectionRef.current?.(true)
    }, delayMs)
  }, [])

  const startRTC = useCallback(async () => {
    if (peerRef.current) return
    setRtcError('')
    const ws = wsChatRef.current
    if (!ws) return

    const peer = new RTCPeerConnection()
    peerRef.current = peer
    peer.onicecandidate = (event) => {
      if (!event.candidate) return
      ws.send({ type: 'webrtc.ice', candidate: event.candidate.toJSON() })
    }
    peer.onconnectionstatechange = () => {
      const state = peer.connectionState
      if (state === 'connected') {
        setRtcStatus('接続済み')
      } else if (state === 'connecting') {
        setRtcStatus('接続中')
      } else if (state === 'failed') {
        setRtcStatus('失敗')
      } else if (state === 'disconnected') {
        setRtcStatus('切断')
      }
    }


    let stream: MediaStream
    try {
      stream = await navigator.mediaDevices.getUserMedia({
        audio: {
          echoCancellation: true,
          noiseSuppression: true,
          autoGainControl: true,
        },
      })
    } catch (err) {
      setRtcError(err instanceof Error ? err.message : 'microphone error')
      setSttStatus('エラー')
      setSttError(err instanceof Error ? err.message : 'microphone error')
      stopRTC()
      return
    }
    micStreamRef.current = stream
    const audioTrack = stream.getAudioTracks()[0]
    if (!audioTrack) {
      setSttStatus('エラー')
      setSttError('マイクトラックを取得できませんでした')
      stopRTC()
      return
    }
    peer.addTrack(audioTrack, stream)
    setAudioSendStatus('確認中')
    setSpeechDetectStatus('待機中')
    setSttStatus('待機中')
    setSttError('')

    try {
      const offer = await peer.createOffer()
      await peer.setLocalDescription(offer)
      ws.send({ type: 'webrtc.offer', sdp: offer.sdp })
      setRtcStatus('接続中')
    } catch (err) {
      setRtcError(err instanceof Error ? err.message : 'RTC start error')
      stopRTC()
    }
  }, [stopRTC])

  const openConnection = useCallback(async (isAutoReconnect: boolean) => {
    if (connected || busy) return
    setBusy(true)
    try {
      // 世代番号はサーバーの再起動で戻るため、比較範囲を接続ごとに区切る。
      currentGenerationRef.current = 0
      resetConversationBubble()
      const wsChat = createWS(chatWSUrl)
      await wsChat.connect((msg) => {
        handleChatMessage(msg)
      }, () => {
        stopRTC()
        setConnected(false)
        setIsAiSpeaking(false)
        if (wsChatRef.current === wsChat) {
          wsChatRef.current = null
        }
        scheduleReconnect()
      })

      wsChatRef.current = wsChat
      audioReceiverRef.current = createAudioReceiver(setIsAiSpeaking, setPlaybackSpeech)
      audioReceiverRef.current.volume(selectedPlaybackVolume.gain)
      setConnected(true)
      reconnectAttemptRef.current = 0
      clearReconnectTimer()
      await startRTC()
      if (isAutoReconnect) {
        console.log('[ws reconnect] connected')
      }
    } catch (e) {
      console.error('connect error', e)
      stopRTC()
      setConnected(false)
      wsChatRef.current?.close()
      wsChatRef.current = null
      if (isAutoReconnect) {
        scheduleReconnect()
        return
      }
      throw e
    } finally {
      setBusy(false)
    }
  }, [appendMessage, busy, clearReconnectTimer, connected, handleChatMessage, resetConversationBubble, scheduleReconnect, selectedPlaybackVolume.gain, startRTC, stopRTC])

  openConnectionRef.current = openConnection

  const connect = useCallback(async () => {
    manualDisconnectRef.current = false
    reconnectAttemptRef.current = 0
    clearReconnectTimer()
    await openConnection(false)
  }, [clearReconnectTimer, openConnection])

  const disconnect = useCallback(() => {
    manualDisconnectRef.current = true
    reconnectAttemptRef.current = 0
    clearReconnectTimer()
    stopRTC()
    wsChatRef.current?.close()
    wsChatRef.current = null
    setConnected(false)
    resetConversationBubble()
  }, [clearReconnectTimer, resetConversationBubble, stopRTC])

  useEffect(() => {
    return () => {
      disconnect()
    }
  }, [disconnect])

  useEffect(() => {
    if (!connected) {
      setAudioSendStatus('停止中')
      lastAudioBytesSentRef.current = null
      return
    }
    const timer = window.setInterval(async () => {
      const peer = peerRef.current
      if (!peer || peer.connectionState !== 'connected') {
        setAudioSendStatus('確認中')
        lastAudioBytesSentRef.current = null
        return
      }
      try {
        const stats = await peer.getStats()
        let currentBytesSent: number | null = null
        stats.forEach((report) => {
          if (report.type !== 'outbound-rtp') return
          const mediaType = (report as RTCOutboundRtpStreamStats).kind || (report as RTCOutboundRtpStreamStats & { mediaType?: string }).mediaType
          if (mediaType !== 'audio') return
          const bytesSent = (report as RTCOutboundRtpStreamStats).bytesSent
          if (typeof bytesSent !== 'number') return
          currentBytesSent = bytesSent
        })
        if (currentBytesSent === null) {
          setAudioSendStatus('確認中')
          lastAudioBytesSentRef.current = null
          return
        }
        if (lastAudioBytesSentRef.current === null) {
          setAudioSendStatus('待機中')
        } else if (currentBytesSent > lastAudioBytesSentRef.current) {
          setAudioSendStatus('送信中')
        } else {
          setAudioSendStatus('待機中')
        }
        lastAudioBytesSentRef.current = currentBytesSent
      } catch (err) {
        setAudioSendStatus('確認失敗')
        setRtcError(err instanceof Error ? err.message : 'RTC stats error')
      }
    }, 1000)
    return () => {
      window.clearInterval(timer)
    }
  }, [connected])


  useEffect(() => {
    const el = adminLogRef.current
    if (el && shouldFollowAdminLogRef.current) {
      el.scrollTop = el.scrollHeight
    }
  }, [serverEventLogs])
  const bubble = useMemo(() => conversationBubble(messages, playbackSpeech), [messages, playbackSpeech])
  const lastAssistantMessage = bubble.assistant
  const lastUserMessage = bubble.user
  const [uiMode, setUiMode] = useState<UiMode>(() => {
    const params = new URLSearchParams(window.location.search)
    if (params.get('ui') === 'jobs' || window.location.pathname.startsWith('/jobs')) return 'jobs'
    if (params.get('ui') === 'admin') return 'admin'
    if (params.get('ui') === 'memory') return 'memory'
    if (params.get('ui') === 'scenes') return 'scenes'
    if (window.location.pathname.startsWith('/admin')) return 'admin'
    if (window.location.pathname.startsWith('/memory')) return 'memory'
    if (window.location.pathname.startsWith('/scenes')) return 'scenes'
    return 'app'
  })
  useEffect(() => {
    const onPopState = () => {
      const params = new URLSearchParams(window.location.search)
      if (params.get('ui') === 'jobs' || window.location.pathname.startsWith('/jobs')) {setUiMode('jobs');return}
      if (params.get('ui') === 'admin') {
        setUiMode('admin')
        return
      }
      if (params.get('ui') === 'memory') {
        setUiMode('memory')
        return
      }
      if (params.get('ui') === 'scenes') {
        setUiMode('scenes')
        return
      }
      if (window.location.pathname.startsWith('/admin')) {
        setUiMode('admin')
        return
      }
      if (window.location.pathname.startsWith('/memory')) {
        setUiMode('memory')
        return
      }
      if (window.location.pathname.startsWith('/scenes')) {
        setUiMode('scenes')
        return
      }
      setUiMode('app')
    }
    window.addEventListener('popstate', onPopState)
    return () => window.removeEventListener('popstate', onPopState)
  }, [])
  useEffect(() => {
    const body = document.body
    if (uiMode !== 'app') {
      body.classList.add('admin-mode')
    } else {
      body.classList.remove('admin-mode')
    }
    return () => body.classList.remove('admin-mode')
  }, [uiMode])
  useEffect(() => {
    if (uiMode !== 'admin' || !shouldFollowAdminLogRef.current) return
    const frame = window.requestAnimationFrame(() => {
      const el = adminLogRef.current
      if (el) {
        el.scrollTop = el.scrollHeight
      }
    })
    return () => window.cancelAnimationFrame(frame)
  }, [uiMode])
  useEffect(() => {
    if (uiMode !== 'memory') return
    const controller = new AbortController()
    void loadMemories(controller.signal)
    return () => controller.abort()
  }, [loadMemories, uiMode])
  useEffect(() => {
    if (uiMode !== 'scenes') return
    const controller = new AbortController()
    void loadSwitchBotScenes(controller.signal)
    return () => controller.abort()
  }, [loadSwitchBotScenes, uiMode])
  const writeMemory = useCallback(async (method: 'POST' | 'PUT', content: string, id?: string) => {
    setMemoryError('')
    const path = id ? `/api/memories/${encodeURIComponent(id)}` : '/api/memories'
    const resp = await fetch(new URL(path, backendURL).toString(), {
      method,
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ content }),
    })
    if (!resp.ok) {
      throw new Error(`メモリを保存できませんでした (${resp.status})`)
    }
    const data = await resp.json() as MemoryWriteResponse
    return normalizeMemoryItem(data.memory)
  }, [])
  const createMemory = useCallback(async (content: string) => {
    try {
      const memory = await writeMemory('POST', content)
      setMemories((prev) => [memory, ...prev.filter((item) => item.id !== memory.id)])
    } catch (err) {
      setMemoryError(err instanceof Error ? err.message : 'メモリを追加できませんでした')
      throw err
    }
  }, [writeMemory])
  const updateMemory = useCallback(async (id: string, content: string) => {
    try {
      const memory = await writeMemory('PUT', content, id)
      setMemories((prev) => prev.map((item) => item.id === id ? memory : item))
    } catch (err) {
      setMemoryError(err instanceof Error ? err.message : 'メモリを更新できませんでした')
      throw err
    }
  }, [writeMemory])
  const deleteMemory = useCallback(async (id: string) => {
    try {
      setMemoryError('')
      const resp = await fetch(new URL(`/api/memories/${encodeURIComponent(id)}`, backendURL).toString(), { method: 'DELETE' })
      if (!resp.ok) {
        throw new Error(`メモリを削除できませんでした (${resp.status})`)
      }
      setMemories((prev) => prev.filter((item) => item.id !== id))
    } catch (err) {
      setMemoryError(err instanceof Error ? err.message : 'メモリを削除できませんでした')
      throw err
    }
  }, [])
  const executeSwitchBotScene = useCallback(async (scene: SwitchBotSceneItem) => {
    try {
      setSwitchBotSceneError('')
      const resp = await fetch(new URL(`/api/switchbot/scenes/${encodeURIComponent(scene.id)}/execute`, backendURL).toString(), { method: 'POST' })
      if (!resp.ok) {
        throw new Error(`シーンを実行できませんでした (${resp.status})`)
      }
    } catch (err) {
      setSwitchBotSceneError(err instanceof Error ? err.message : 'シーンを実行できませんでした')
      throw err
    }
  }, [])
  const setMode = useCallback((mode: UiMode) => {
    const params = new URLSearchParams(window.location.search)
    if (mode === 'jobs') {params.set('ui', 'jobs')} else if (mode === 'admin') {
      params.set('ui', 'admin')
    } else if (mode === 'memory') {
      params.set('ui', 'memory')
    } else if (mode === 'scenes') {
      params.set('ui', 'scenes')
    } else {
      params.delete('ui')
    }
    const next = params.toString()
    const nextUrl = next ? `/?${next}` : '/'
    window.history.pushState({}, '', nextUrl)
    setUiMode(mode)
  }, [])

  return (
    <>
      <div style={{ display: uiMode === 'app' ? 'block' : 'none' }}>
        <LiveView
          connected={connected}
          connecting={busy}
          speechDetectStatus={speechDetectStatus}
          sttStatus={sttStatus}
          lastAssistantMessage={lastAssistantMessage}
          lastUserMessage={lastUserMessage}
          isAiSpeaking={isAiSpeaking}
          isConversationBubbleHidden={!bubble.visible}
          boardEntries={boardEntries}
          toolToasts={toolToasts}
          playbackVolumeLevel={playbackVolumeLevel}
          onPlaybackVolumeChange={setPlaybackVolumeLevel}
          connect={connect}
          disconnect={disconnect}
          goScenes={() => setMode('scenes')}
          goMemory={() => setMode('memory')}
          goAdmin={() => setMode('admin')}
          goJobs={() => setMode('jobs')}
        />
      </div>
      <div
        style={{
          display: uiMode === 'admin' ? 'flex' : 'none',
          flexDirection: 'column',
          gap: 12,
          padding: 12,
          height: '100vh',
          minHeight: 0,
          overflow: 'hidden',
          boxSizing: 'border-box',
        }}
      >
        <div
          style={{
            flex: '1 1 auto',
            minHeight: 0,
            height: '100%',
            border: '2px solid var(--live-line)',
            borderRadius: 10,
            padding: '10px 12px',
            overflow: 'auto',
            background: 'var(--live-panel-soft)',
            boxShadow: 'inset 0 0 0 1px var(--live-inset-line)',
            fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", monospace',
            fontSize: 12,
            lineHeight: 1.5,
          }}
          ref={adminLogRef}
          onScroll={updateAdminLogFollow}
        >
          {serverEventLogs.map((log) => (
            <div key={log.id} style={{ whiteSpace: 'pre', minWidth: 'max-content' }}>
              {log.line}
            </div>
          ))}
        </div>
        <button
          onClick={() => setMode('app')}
          style={{
            alignSelf: 'flex-start',
            borderRadius: 8,
            border: '1px solid var(--live-line-strong)',
            background: 'var(--live-panel)',
            color: 'var(--live-text)',
            padding: '10px 14px',
            fontSize: 14,
            fontWeight: 700,
            cursor: 'pointer',
          }}
        >
          app画面
        </button>
      </div>
      <AutomationView visible={uiMode === 'jobs'} backendURL={backendURL} goApp={() => setMode('app')} />
      <MemoryView
        visible={uiMode === 'memory'}
        memories={memories}
        loading={memoryLoading}
        error={memoryError}
        onCreate={createMemory}
        onUpdate={updateMemory}
        onDelete={deleteMemory}
        goApp={() => setMode('app')}
      />
      <SwitchBotSceneView
        visible={uiMode === 'scenes'}
        scenes={switchBotScenes}
        loading={switchBotSceneLoading}
        error={switchBotSceneError}
        onExecute={executeSwitchBotScene}
        goApp={() => setMode('app')}
      />
    </>
  )
}

function MemoryView(props: {
  visible: boolean
  memories: MemoryItem[]
  loading: boolean
  error: string
  onCreate: (content: string) => Promise<void>
  onUpdate: (id: string, content: string) => Promise<void>
  onDelete: (id: string) => Promise<void>
  goApp: () => void
}) {
  const { visible, memories, loading, error, onCreate, onUpdate, onDelete, goApp } = props
  const [newContent, setNewContent] = useState('')
  const [editingID, setEditingID] = useState('')
  const [editingContent, setEditingContent] = useState('')
  const [pendingAction, setPendingAction] = useState('')
  const startEdit = useCallback((memory: MemoryItem) => {
    setEditingID(memory.id)
    setEditingContent(memory.content)
  }, [])
  const cancelEdit = useCallback(() => {
    setEditingID('')
    setEditingContent('')
  }, [])
  const submitNew = useCallback(async () => {
    const content = newContent.trim()
    if (!content) return
    setPendingAction('create')
    try {
      await onCreate(content)
      setNewContent('')
    } finally {
      setPendingAction('')
    }
  }, [newContent, onCreate])
  const submitEdit = useCallback(async (id: string) => {
    const content = editingContent.trim()
    if (!content) return
    setPendingAction(`update:${id}`)
    try {
      await onUpdate(id, content)
      cancelEdit()
    } finally {
      setPendingAction('')
    }
  }, [cancelEdit, editingContent, onUpdate])
  const submitDelete = useCallback(async (memory: MemoryItem) => {
    if (!window.confirm('このメモリを削除しますか？')) return
    setPendingAction(`delete:${memory.id}`)
    try {
      await onDelete(memory.id)
      if (editingID === memory.id) {
        cancelEdit()
      }
    } finally {
      setPendingAction('')
    }
  }, [cancelEdit, editingID, onDelete])
  return (
    <div
      style={{
        display: visible ? 'flex' : 'none',
        flexDirection: 'column',
        gap: 12,
        padding: 12,
        height: '100vh',
        minHeight: 0,
        overflow: 'hidden',
        boxSizing: 'border-box',
      }}
    >
      {error && (
        <div
          style={{
            border: '1px solid var(--ui-danger-line)',
            borderRadius: 8,
            background: 'var(--ui-danger-bg)',
            color: 'var(--ui-danger-text)',
            padding: '10px 12px',
            fontSize: 13,
            lineHeight: 1.4,
            fontWeight: 700,
          }}
        >
          {error}
        </div>
      )}
      <div
        style={{
          flex: '1 1 auto',
          minHeight: 0,
          overflow: 'auto',
          border: '2px solid var(--live-line)',
          borderRadius: 10,
          background: 'var(--live-panel-soft)',
          padding: 12,
        }}
      >
        <div style={{ display: 'grid', gap: 10 }}>
          <div
            style={{
              display: 'grid',
              gap: 8,
              border: '2px solid var(--live-line)',
              borderRadius: 8,
              background: 'var(--live-panel)',
              padding: 12,
            }}
          >
            <textarea
              value={newContent}
              onChange={(event) => setNewContent(event.currentTarget.value)}
              placeholder="追加するメモリ文"
              rows={3}
              style={{
                width: '100%',
                resize: 'vertical',
                border: '1px solid var(--live-line-strong)',
                borderRadius: 8,
                padding: '10px 12px',
                color: 'var(--live-text)',
                fontSize: 14,
                lineHeight: 1.5,
                fontFamily: 'inherit',
              }}
            />
            <button
              onClick={() => void submitNew()}
              disabled={!newContent.trim() || pendingAction === 'create'}
              style={{
                justifySelf: 'start',
                borderRadius: 8,
                border: '1px solid var(--ui-accent)',
                background: !newContent.trim() || pendingAction === 'create' ? 'var(--ui-accent-soft)' : 'var(--ui-accent)',
                color: !newContent.trim() || pendingAction === 'create' ? 'var(--ui-accent-soft-text)' : 'var(--ui-accent-text)',
                padding: '9px 13px',
                fontSize: 14,
                fontWeight: 800,
                cursor: !newContent.trim() || pendingAction === 'create' ? 'not-allowed' : 'pointer',
              }}
            >
              {pendingAction === 'create' ? '追加中' : '追加'}
            </button>
          </div>
          {!loading && !error && memories.length === 0 && (
            <div style={{ color: 'var(--live-muted)', fontSize: 14, fontWeight: 700 }}>保存済みメモリはありません</div>
          )}
          {memories.map((memory) => (
            <div
              key={memory.id}
              style={{
                border: '1px solid var(--live-line)',
                borderRadius: 8,
                background: 'var(--live-panel)',
                padding: 12,
                display: 'grid',
                gap: 8,
              }}
            >
              {editingID === memory.id ? (
                <textarea
                  value={editingContent}
                  onChange={(event) => setEditingContent(event.currentTarget.value)}
                  rows={4}
                  style={{
                    width: '100%',
                    resize: 'vertical',
                    border: '1px solid var(--live-line-strong)',
                    borderRadius: 8,
                    padding: '10px 12px',
                    color: 'var(--live-text)',
                    fontSize: 14,
                    lineHeight: 1.5,
                    fontFamily: 'inherit',
                  }}
                />
              ) : (
                <div style={{ color: 'var(--live-text)', fontSize: 14, lineHeight: 1.6, whiteSpace: 'pre-wrap' }}>
                  {memory.content}
                </div>
              )}
              {memory.tags.length > 0 && (
                <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
                  {memory.tags.map((tag) => (
                    <span
                      key={`${memory.id}-${tag}`}
                      style={{
                        border: '1px solid var(--live-line-strong)',
                        borderRadius: 999,
                        padding: '3px 8px',
                        background: 'var(--live-panel-soft)',
                        color: 'var(--live-muted)',
                        fontSize: 12,
                        lineHeight: 1.2,
                        fontWeight: 700,
                      }}
                    >
                      {tag}
                    </span>
                  ))}
                </div>
              )}
              <div style={{ display: 'flex', gap: 12, flexWrap: 'wrap', color: 'var(--live-muted)', fontSize: 12 }}>
                <span>作成: {formatMemoryDate(memory.created_at)}</span>
                <span>更新: {formatMemoryDate(memory.updated_at)}</span>
              </div>
              <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                {editingID === memory.id ? (
                  <>
                    <button
                      onClick={() => void submitEdit(memory.id)}
                      disabled={!editingContent.trim() || pendingAction === `update:${memory.id}`}
                      style={{
                        borderRadius: 8,
                        border: '1px solid var(--ui-accent)',
                        background: !editingContent.trim() || pendingAction === `update:${memory.id}` ? 'var(--ui-accent-soft)' : 'var(--ui-accent)',
                        color: !editingContent.trim() || pendingAction === `update:${memory.id}` ? 'var(--ui-accent-soft-text)' : 'var(--ui-accent-text)',
                        padding: '8px 12px',
                        fontSize: 13,
                        fontWeight: 800,
                        cursor: !editingContent.trim() || pendingAction === `update:${memory.id}` ? 'not-allowed' : 'pointer',
                      }}
                    >
                      {pendingAction === `update:${memory.id}` ? '保存中' : '保存'}
                    </button>
                    <button
                      onClick={cancelEdit}
                      disabled={pendingAction === `update:${memory.id}`}
                      style={{
                        borderRadius: 8,
                        border: '1px solid var(--live-line-strong)',
                        background: 'var(--live-panel)',
                        color: 'var(--live-text)',
                        padding: '8px 12px',
                        fontSize: 13,
                        fontWeight: 800,
                        cursor: pendingAction === `update:${memory.id}` ? 'not-allowed' : 'pointer',
                      }}
                    >
                      キャンセル
                    </button>
                  </>
                ) : (
                  <>
                    <button
                      onClick={() => startEdit(memory)}
                      disabled={pendingAction !== ''}
                      style={{
                        borderRadius: 8,
                        border: '1px solid var(--live-line-strong)',
                        background: 'var(--live-panel)',
                        color: 'var(--live-text)',
                        padding: '8px 12px',
                        fontSize: 13,
                        fontWeight: 800,
                        cursor: pendingAction !== '' ? 'not-allowed' : 'pointer',
                      }}
                    >
                      編集
                    </button>
                    <button
                      onClick={() => void submitDelete(memory)}
                      disabled={pendingAction === `delete:${memory.id}`}
                      style={{
                        borderRadius: 8,
                        border: '1px solid var(--ui-danger-line)',
                        background: 'var(--ui-danger-bg)',
                        color: 'var(--ui-danger-text)',
                        padding: '8px 12px',
                        fontSize: 13,
                        fontWeight: 800,
                        cursor: pendingAction === `delete:${memory.id}` ? 'not-allowed' : 'pointer',
                      }}
                    >
                      {pendingAction === `delete:${memory.id}` ? '削除中' : '削除'}
                    </button>
                  </>
                )}
              </div>
            </div>
          ))}
        </div>
      </div>
      <button
        onClick={goApp}
        style={{
          alignSelf: 'flex-start',
          borderRadius: 8,
          border: '1px solid var(--live-line-strong)',
          background: 'var(--live-panel)',
          color: 'var(--live-text)',
          padding: '10px 14px',
          fontSize: 14,
          fontWeight: 700,
          cursor: 'pointer',
        }}
      >
        app画面
      </button>
    </div>
  )
}

function SwitchBotSceneView(props: {
  visible: boolean
  scenes: SwitchBotSceneItem[]
  loading: boolean
  error: string
  onExecute: (scene: SwitchBotSceneItem) => Promise<void>
  goApp: () => void
}) {
  const { visible, scenes, loading, error, onExecute, goApp } = props
  const [pendingSceneID, setPendingSceneID] = useState('')
  const sortedScenes = useMemo(() => {
    return [...scenes].sort((a, b) => a.name.localeCompare(b.name, 'ja'))
  }, [scenes])
  const submitScene = useCallback(async (scene: SwitchBotSceneItem) => {
    setPendingSceneID(scene.id)
    try {
      await onExecute(scene)
    } finally {
      setPendingSceneID('')
    }
  }, [onExecute])
  return (
    <div
      style={{
        display: visible ? 'flex' : 'none',
        flexDirection: 'column',
        gap: 12,
        padding: 12,
        height: '100vh',
        minHeight: 0,
        overflow: 'hidden',
        boxSizing: 'border-box',
      }}
    >
      <style>{`
        .switchbot-scene-grid {
          display: grid;
          grid-template-columns: repeat(2, minmax(0, 1fr));
          gap: 10px;
        }
        .switchbot-scene-button {
          min-width: 0;
          min-height: 42px;
          border-radius: 8px;
          border: 1px solid var(--live-line-strong);
          padding: 4px 8px;
          font-size: 21px;
          font-weight: 800;
          line-height: 1.2;
          white-space: normal;
          overflow-wrap: anywhere;
          word-break: break-word;
          box-shadow: 0 1px 2px rgba(15, 23, 42, 0.04);
        }
        @media (orientation: portrait) {
          .switchbot-scene-grid {
            grid-template-columns: minmax(0, 1fr);
          }
        }
      `}</style>
      {error && (
        <div
          style={{
            border: '1px solid var(--ui-danger-line)',
            borderRadius: 8,
            background: 'var(--ui-danger-bg)',
            color: 'var(--ui-danger-text)',
            padding: '10px 12px',
            fontSize: 13,
            lineHeight: 1.4,
            fontWeight: 700,
          }}
        >
          {error}
        </div>
      )}
      <div
        style={{
          flex: '1 1 auto',
          minHeight: 0,
          overflow: 'auto',
          border: '2px solid var(--live-line)',
          borderRadius: 10,
          background: 'var(--live-panel-soft)',
          padding: 12,
        }}
      >
        {loading && (
          <div style={{ color: 'var(--live-muted)', fontSize: 14, fontWeight: 700 }}>シーンを読み込み中です</div>
        )}
        {!loading && !error && scenes.length === 0 && (
          <div style={{ color: 'var(--live-muted)', fontSize: 14, fontWeight: 700 }}>利用できるシーンはありません</div>
        )}
        <div
          className="switchbot-scene-grid"
        >
          {sortedScenes.map((scene) => {
            const pending = pendingSceneID === scene.id
            return (
              <button
                key={scene.id}
                onClick={() => void submitScene(scene)}
                disabled={pendingSceneID !== ''}
                className="switchbot-scene-button"
                style={{
                  background: pending ? 'var(--ui-accent-soft)' : 'var(--live-panel)',
                  color: pending ? 'var(--ui-accent-soft-text)' : 'var(--live-text)',
                  cursor: pendingSceneID !== '' ? 'not-allowed' : 'pointer',
                }}
              >
                {pending ? '実行中' : scene.name}
              </button>
            )
          })}
        </div>
      </div>
      <button
        onClick={goApp}
        style={{
          alignSelf: 'flex-start',
          borderRadius: 8,
          border: '1px solid var(--live-line-strong)',
          background: 'var(--live-panel)',
          color: 'var(--live-text)',
          padding: '10px 14px',
          fontSize: 14,
          fontWeight: 700,
          cursor: 'pointer',
        }}
      >
        app画面
      </button>
    </div>
  )
}

type TTSSelection = {
  provider: string
  providers: { id: string; label: string; available: boolean }[]
}

function TTSControl() {
  const [selection, setSelection] = useState<TTSSelection | null>(null)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')
  const requestID = useRef(0)
  const request = useCallback(async (provider?: string) => {
    const id = ++requestID.current
    setPending(true)
    setError('')
    try {
      const response = await fetch(new URL('/api/tts', backendURL), {
        method: provider === undefined ? 'GET' : 'PUT',
        headers: provider === undefined ? undefined : { 'Content-Type': 'application/json' },
        body: provider === undefined ? undefined : JSON.stringify({ provider }),
        cache: 'no-store',
      })
      if (!response.ok) throw new Error(await response.text() || 'TTSの設定を取得・変更できませんでした')
      const next = await response.json() as TTSSelection
      if (id === requestID.current) setSelection(next)
    } catch (err) {
      if (id === requestID.current) setError(err instanceof Error ? err.message : 'TTSを変更できませんでした')
    } finally {
      if (id === requestID.current) setPending(false)
    }
  }, [])
  useEffect(() => {
    const refresh = () => { void request() }
    refresh()
    window.addEventListener('focus', refresh)
    return () => {
      window.removeEventListener('focus', refresh)
      requestID.current++
    }
  }, [request])
  return (
    <div style={{ display: 'grid', gap: 4, fontSize: 12 }}>
      <label style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
        <select
          aria-label="読み上げ音声"
          value={selection?.provider ?? ''}
          disabled={pending || !selection}
          onChange={(event) => void request(event.currentTarget.value)}
          style={{ background: 'var(--live-panel-soft)', color: 'var(--live-text)', border: '1px solid var(--live-line)', borderRadius: 6, padding: 4 }}
        >
          {!selection && <option value="">{pending ? '読み込み中' : '取得できません'}</option>}
          {selection?.providers.map((provider) => (
            <option key={provider.id} value={provider.id} disabled={!provider.available}>
              {provider.label}{provider.available ? '' : '（未設定）'}
            </option>
          ))}
        </select>
        {error && <button className="live-admin-btn" onClick={() => void request()} disabled={pending}>再取得</button>}
      </label>
      {error && <span role="alert">{error}</span>}
    </div>
  )
}

function LiveView(props: LiveViewProps) {
  const {
    connected,
    connecting,
    speechDetectStatus,
    sttStatus,
    lastAssistantMessage,
    lastUserMessage,
    isAiSpeaking,
    isConversationBubbleHidden,
    boardEntries,
    toolToasts,
    playbackVolumeLevel,
    onPlaybackVolumeChange,
    connect,
    disconnect,
    goScenes,
    goMemory,
    goAdmin,
    goJobs,
  } = props
  const handleToggle = useCallback(() => {
    if (connected) {
      disconnect()
      return
    }
    void connect()
  }, [connected, connect, disconnect])
  const connectionStatus = connecting ? '接続中' : connected ? 'オンライン' : 'オフライン'
  const playbackVolumeIndex = playbackVolumeLevels.indexOf(playbackVolumeLevel)
  const selectedPlaybackVolume = playbackVolumePresets[playbackVolumeLevel]
  return (
    <>
      <style>{liveRootStyle}</style>
      <div className="live-frame">
        <div className="live-main">
          <div className="live-left">
            <Whiteboard entries={boardEntries} backendURL={backendURL} />
            <div className="live-bubble-slot">
              {(lastUserMessage || lastAssistantMessage) && !isConversationBubbleHidden && (
                <div className={`live-bubble${isAiSpeaking ? ' live-bubble-ai-speaking' : ''}`}>
                  <div className="live-bubble-content">
                    {lastUserMessage && <div className="live-bubble-user">{lastUserMessage}</div>}
                    {lastAssistantMessage && <div className="live-bubble-agent">{lastAssistantMessage}</div>}
                  </div>
                </div>
              )}
            </div>
          </div>
          <div className="live-right">
            <div className="live-controls-panel">
              <div className="live-controls-row">
                <div className="live-controls-actions">
                  <button onClick={handleToggle} className="live-control-btn" aria-label="接続切替">
                    <span className={`live-toggle-switch ${connected ? 'on' : ''}`}></span>
                    接続
                  </button>
                  <button onClick={goScenes} className="live-admin-btn">リモコン</button>
                  <button onClick={goMemory} className="live-admin-btn">メモリ</button>
                  <button onClick={goAdmin} className="live-admin-btn">ログ</button>
                  <button onClick={goJobs} className="live-admin-btn">予約</button>
                </div>
              </div>
              <div className="live-status-grid">
                <div className="live-status-card">
                  <div className="live-status-label">接続</div>
                  <div className="live-status-value">{connectionStatus}</div>
                </div>
                <div className="live-status-card">
                  <div className="live-status-label">マイク</div>
                  <div className="live-status-value">{speechDetectStatus}</div>
                </div>
                <div className="live-status-card">
                  <div className="live-status-label">認識</div>
                  <div className="live-status-value">{sttStatus}</div>
                </div>
              </div>
              <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 8, minWidth: 0 }}>
                <TTSControl />
                <WakeModeControl backendURL={backendURL} />
              </div>
              <div className="live-playback-slider-row">
                <span className="live-playback-slider-label">再生音量</span>
                <input
                  className="live-volume-slider"
                  type="range"
                  min={0}
                  max={playbackVolumeLevels.length - 1}
                  step={1}
                  value={playbackVolumeIndex}
                  aria-label="再生音量"
                  onChange={(event) => onPlaybackVolumeChange(playbackVolumeLevels[Number(event.currentTarget.value)])}
                />
              </div>
            </div>
            <div className="live-character-area">
              <div className="live-volume-control" aria-label="キャラクターエリア">
                <div className="live-tool-toast-stack">
                  {toolToasts.map((toast) => (
                    <div key={toast.id} className={`live-tool-toast ${toast.kind}`}>
                      <span className="live-tool-toast-tool">{toast.kind === 'call' ? 'call' : 'result'}: {toast.toolName}</span>
                    </div>
                  ))}
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </>
  )
}

const rootEl = document.getElementById('root')
if (!rootEl) {
  throw new Error('root element not found')
}
if ('serviceWorker' in navigator) {
  window.addEventListener('load', () => {
    navigator.serviceWorker.register('/sw.js').catch((err) => {
      console.warn('service worker登録に失敗しました', err)
    })
  })
}
ReactDOM.createRoot(rootEl).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
)
