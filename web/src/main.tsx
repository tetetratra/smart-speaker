import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import ReactDOM from 'react-dom/client'
import { createWS } from './ws'
import { conversationBubble } from './conversation'
import { Whiteboard, type HistoryEntry } from './Whiteboard'
import { historyEntries, mergeRecords, splitRecords, type WhiteboardRecord } from './whiteboardHistory'
import { useWakeMode, type WakeMode } from './WakeModeControl'
import { SettingsView } from './SettingsView'
import { AutomationView } from './AutomationView'
import { defaultPlaybackVolumeLevel, playbackVolumeGains, usePlaybackVolume, type PlaybackVolumeLevel } from './plugins/playbackvolume/playbackVolume'
import { SwitchBotSceneView } from './plugins/switchbot/SwitchBotSceneView'
import { createAudioReceiver, type AudioReceiver, type PlaybackSpeech } from './audio'
import { startBubbleAutoScroll } from './bubbleScroll'
import { apiURL, wsURL } from './shared/backend'
import './shared/theme.css'
import { useScreen } from './app/routes'

// ChatMessage はチャット欄（吹き出し）に出すユーザーの発話。AI の発話は playback_progress から作る。
type ChatMessage = { id: number; type: 'user'; text: string; generationId?: number }

type ServerEventLog = { id: number; line: string }
type MemoryItem = {
  id: string
  content: string
  created_at: string
  updated_at: string
}
type MemoryListResponse = { memories: MemoryItem[] }
type MemoryWriteResponse = { memory: MemoryItem }

const backendURL = new URL(apiURL('/'))
const chatWSUrl = wsURL('/ws/chat')
const reconnectMaxAttempts = 10
const reconnectInitialDelayMs = 1000
const serverEventLogLimit = 1000

const liveRootStyle = `
  .live-frame {
    width: 100vw;
    height: 100vh;
    height: 100dvh;
    padding: 6px;
    position: relative;
    background: var(--live-bg);
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
    grid-template-columns: minmax(0, 1fr) minmax(220px, 30%);
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
    grid-template-rows: auto 1fr;
    gap: 8px;
  }
  .live-controls-toggles,
  .live-controls-nav {
    display: grid;
    gap: 4px;
    width: 100%;
    min-width: 0;
  }
  .live-controls-toggles {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .live-controls-nav {
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }
  .live-controls-toggles .live-control-btn,
  .live-controls-nav .live-admin-btn {
    width: 100%;
    min-width: 0;
    padding-inline: 4px;
  }
  .live-controls-toggles .live-control-btn {
    justify-content: center;
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
    transition: background-color 0.3s;
  }
  /* 電源（接続）がオフの間と、呼びかけ待ちの間は、話しかけても反応しないことが分かるよう暗くする。 */
  .live-volume-control.live-character-dim {
    background: var(--live-character-dim);
  }
  .live-tool-toast {
    min-height: 24px;
    border: 1px solid transparent;
    border-radius: 6px;
    padding: 3px 8px;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    font-size: 12px;
    line-height: 1.2;
    font-weight: 700;
    transition: background-color 0.3s, border-color 0.3s, color 0.3s;
  }
  .live-tool-toast.tool {
    background: var(--ui-tool-done-bg);
    border-color: var(--ui-tool-done-line);
    color: var(--ui-tool-done-text);
  }
  .live-tool-toast-tool {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
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
    font-size: 9px;
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
    font-size: 20px;
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
    grid-template-rows: auto auto;
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
      grid-template-rows: auto minmax(0, 3fr) minmax(0, 3fr) minmax(0, 5fr);
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
      font-size: clamp(6px, 1.6vw, 8px);
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
    created_at: String(memory.created_at),
    updated_at: String(memory.updated_at),
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
  // 吹き出しに表示中の読み上げ区間。区間が変わるたびに再生時間に合わせて自動スクロールする。
  assistantSpeech?: PlaybackSpeech
  // 今再生中の読み上げ区間。ホワイトボードのスクロールの速さを読み上げに合わせるのに使う。
  playbackSpeech?: PlaybackSpeech
  toolEntries: HistoryEntry[]
  wakeMode?: WakeMode
  wake: () => Promise<void>
  connect: () => Promise<void>
  disconnect: () => void
  goScenes: () => void
  goMemory: () => void
  goAdmin: () => void
  goJobs: () => void
  goSettings: () => void
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
  const [playbackVolumeLevel, setPlaybackVolumeLevel] = usePlaybackVolume()
  const wakeMode = useWakeMode()
  const [whiteboardRecords, setWhiteboardRecords] = useState<Map<string, WhiteboardRecord>>(() => new Map())
  const toolEntries = useMemo(() => historyEntries(whiteboardRecords.values()), [whiteboardRecords])
  const [serverEventLogs, setServerEventLogs] = useState<ServerEventLog[]>([])
  const [memories, setMemories] = useState<MemoryItem[]>([])
  const [memoryLoading, setMemoryLoading] = useState(false)
  const [memoryError, setMemoryError] = useState('')
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
  const [isAiSpeaking, setIsAiSpeaking] = useState(false)
  const currentGenerationRef = useRef(0)
  const idRef = useRef(0)
  const adminLogRef = useRef<HTMLDivElement | null>(null)
  const shouldFollowAdminLogRef = useRef(true)
  const audioReceiverRef = useRef<AudioReceiver | null>(null)
  const playbackGainRef = useRef(playbackVolumeGains[defaultPlaybackVolumeLevel])

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

  const playbackGain = playbackVolumeGains[playbackVolumeLevel]

  const stopRemoteAudioGraph = useCallback(() => {
    audioReceiverRef.current?.close()
    audioReceiverRef.current = null
  }, [])

  useEffect(() => {
    playbackGainRef.current = playbackGain
    audioReceiverRef.current?.volume(playbackGain)
  }, [playbackGain])

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
      if (raw.type === 'webrtc.answer' || raw.type === 'webrtc.ice') {
        handleRTCSignal(raw)
        return
      }
      switch (raw.type) {
        case 'playback_progress': {
          audioReceiverRef.current?.progress(raw)
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
        case 'conversation_records': {
          if (!Array.isArray(raw.records)) break
          // 画面に出す記録だけが届くので、どこに出すかだけを決める。
          const { utterances, others } = splitRecords(raw.records as WhiteboardRecord[])
          for (const { text, generationId } of utterances) {
            if (generationId !== undefined) currentGenerationRef.current = Math.max(currentGenerationRef.current, generationId)
            if (generationId === undefined || generationId === currentGenerationRef.current) {
              setIsAiSpeaking(false)
              setSttStatus('完了')
              setSpeechDetectStatus('待機中')
            }
            appendMessage({ id: nextMessageId(), type: 'user', text, generationId })
          }
          if (others.length > 0) setWhiteboardRecords(current => mergeRecords(current, others))
          break
        }
        default:
          break
      }
    },
    [appendMessage, appendServerEventLog, handleRTCSignal, nextMessageId],
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
    peer.ontrack = (event) => {
      if (peerRef.current !== peer || event.track.kind !== 'audio') return
      audioReceiverRef.current?.attach(event.streams[0] ?? new MediaStream([event.track]))
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
      audioReceiverRef.current = createAudioReceiver(setIsAiSpeaking, setPlaybackSpeech, setRtcError)
      audioReceiverRef.current.volume(playbackGainRef.current)
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
  }, [appendMessage, busy, clearReconnectTimer, connected, handleChatMessage, resetConversationBubble, scheduleReconnect, playbackGain, startRTC, stopRTC])

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
  const [screen, navigate] = useScreen()
  useEffect(() => {
    const body = document.body
    if (screen !== 'live') {
      body.classList.add('admin-mode')
    } else {
      body.classList.remove('admin-mode')
    }
    return () => body.classList.remove('admin-mode')
  }, [screen])
  useEffect(() => {
    if (screen !== 'eventlog' || !shouldFollowAdminLogRef.current) return
    const frame = window.requestAnimationFrame(() => {
      const el = adminLogRef.current
      if (el) {
        el.scrollTop = el.scrollHeight
      }
    })
    return () => window.cancelAnimationFrame(frame)
  }, [screen])
  useEffect(() => {
    if (screen !== 'memory') return
    const controller = new AbortController()
    void loadMemories(controller.signal)
    return () => controller.abort()
  }, [loadMemories, screen])
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
  return (
    <>
      <div style={{ display: screen === 'live' ? 'block' : 'none' }}>
        <LiveView
          connected={connected}
          connecting={busy}
          speechDetectStatus={speechDetectStatus}
          sttStatus={sttStatus}
          lastAssistantMessage={lastAssistantMessage}
          lastUserMessage={lastUserMessage}
          isAiSpeaking={isAiSpeaking}
          isConversationBubbleHidden={!bubble.visible}
          assistantSpeech={bubble.assistant ? playbackSpeech : undefined}
          playbackSpeech={playbackSpeech}
          toolEntries={toolEntries}
          wakeMode={wakeMode.mode}
          wake={wakeMode.wake}
          connect={connect}
          disconnect={disconnect}
          goScenes={() => navigate('scenes')}
          goMemory={() => navigate('memory')}
          goAdmin={() => navigate('eventlog')}
          goJobs={() => navigate('jobs')}
          goSettings={() => navigate('settings')}
        />
      </div>
      {screen === 'eventlog' && <div
        style={{
          display: 'flex',
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
          onClick={() => navigate('live')}
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
      </div>}
      {screen === 'jobs' && <AutomationView backendURL={backendURL} goApp={() => navigate('live')} />}
      {screen === 'memory' && <MemoryView
        memories={memories}
        loading={memoryLoading}
        error={memoryError}
        onCreate={createMemory}
        onUpdate={updateMemory}
        onDelete={deleteMemory}
        goApp={() => navigate('live')}
      />}
      {screen === 'scenes' && <SwitchBotSceneView
        backendURL={backendURL}
        goApp={() => navigate('live')}
      />}
      {screen === 'settings' && <SettingsView
        backendURL={backendURL}
        connectionStatus={connectionStatusLabel(connected, busy)}
        speechDetectStatus={speechDetectStatus}
        sttStatus={sttStatus}
        wakeMode={wakeMode}
        playbackVolumeLevel={playbackVolumeLevel}
        onPlaybackVolumeChange={setPlaybackVolumeLevel}
        goApp={() => navigate('live')}
      />}
    </>
  )
}

// 縦画面では幅が狭いので、編集・削除ボタンを縦に並べて本文の幅を確保する。
// 作成日と更新日は折り返さず1行に並べ、1件あたりの高さを抑える。
const memoryViewStyle = `
  .memory-item-dates {
    display: flex;
    column-gap: 12px;
    color: var(--live-muted);
    font-size: 11px;
    white-space: nowrap;
    overflow: hidden;
  }
  .memory-item-actions {
    display: flex;
    gap: 6px;
  }
  @media (orientation: portrait) {
    .memory-item-dates {
      column-gap: 8px;
      font-size: 10px;
    }
    .memory-item-actions {
      flex-direction: column;
    }
  }
`

function MemoryView(props: {
  memories: MemoryItem[]
  loading: boolean
  error: string
  onCreate: (content: string) => Promise<void>
  onUpdate: (id: string, content: string) => Promise<void>
  onDelete: (id: string) => Promise<void>
  goApp: () => void
}) {
  const { memories, loading, error, onCreate, onUpdate, onDelete, goApp } = props
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
        display: 'flex',
        flexDirection: 'column',
        gap: 12,
        padding: 12,
        height: '100vh',
        minHeight: 0,
        overflow: 'hidden',
        boxSizing: 'border-box',
      }}
    >
      <style>{memoryViewStyle}</style>
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
                padding: '8px 12px',
                display: 'grid',
                gridTemplateColumns: 'minmax(0, 1fr) auto',
                columnGap: 12,
                alignItems: 'center',
              }}
            >
              <div style={{ display: 'grid', gap: 2, minWidth: 0 }}>
                {editingID === memory.id ? (
                  <textarea
                    value={editingContent}
                    onChange={(event) => setEditingContent(event.currentTarget.value)}
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
                ) : (
                  <div style={{ color: 'var(--live-text)', fontSize: 14, lineHeight: 1.5, whiteSpace: 'pre-wrap' }}>
                    {memory.content}
                  </div>
                )}
                <div className="memory-item-dates">
                  <span>作成: {formatMemoryDate(memory.created_at)}</span>
                  <span>更新: {formatMemoryDate(memory.updated_at)}</span>
                </div>
              </div>
              <div className="memory-item-actions">
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
                        padding: '6px 10px',
                        fontSize: 13,
                        fontWeight: 800,
                        whiteSpace: 'nowrap',
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
                        padding: '6px 10px',
                        fontSize: 13,
                        fontWeight: 800,
                        whiteSpace: 'nowrap',
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
                        padding: '6px 10px',
                        fontSize: 13,
                        fontWeight: 800,
                        whiteSpace: 'nowrap',
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
                        padding: '6px 10px',
                        fontSize: 13,
                        fontWeight: 800,
                        whiteSpace: 'nowrap',
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

function connectionStatusLabel(connected: boolean, connecting: boolean): string {
  return connecting ? '接続中' : connected ? 'オンライン' : 'オフライン'
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
    assistantSpeech,
    playbackSpeech,
    toolEntries,
    wakeMode,
    wake,
    connect,
    disconnect,
    goScenes,
    goMemory,
    goAdmin,
    goJobs,
    goSettings,
  } = props
  const bubbleContentRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const element = bubbleContentRef.current
    if (!element || !assistantSpeech) return
    return startBubbleAutoScroll(element, assistantSpeech.durationMs)
  }, [assistantSpeech])
  const handleToggle = useCallback(() => {
    if (connected) {
      disconnect()
      return
    }
    void connect()
  }, [connected, connect, disconnect])
  // 呼びかけ状態が分からない間（呼びかけ設定の接続が切れている間）は、呼びかけ待ちとは扱わない。
  const isCharacterDim = !connected || (wakeMode?.required === true && !wakeMode.awake)
  // 暗いとき（話しかけても反応しないとき）にタップすると、接続して呼びかけ中にし、すぐ話せるようにする。
  const handleCharacterTap = useCallback(async () => {
    if (!isCharacterDim) return
    if (!connected) {
      try {
        await connect()
      } catch {
        return
      }
    }
    await wake()
  }, [isCharacterDim, connected, connect, wake])
  return (
    <>
      <style>{liveRootStyle}</style>
      <div className="live-frame">
        <div className="live-main">
          <div className="live-left">
            <Whiteboard tools={toolEntries} speech={playbackSpeech} />
            <div className="live-bubble-slot">
              {(lastUserMessage || lastAssistantMessage) && !isConversationBubbleHidden && (
                <div className={`live-bubble${isAiSpeaking ? ' live-bubble-ai-speaking' : ''}`}>
                  <div className="live-bubble-content" ref={bubbleContentRef}>
                    {lastUserMessage && <div className="live-bubble-user">{lastUserMessage}</div>}
                    {lastAssistantMessage && <div className="live-bubble-agent">{lastAssistantMessage}</div>}
                  </div>
                </div>
              )}
            </div>
          </div>
          <div className="live-right">
            <div className="live-controls-panel">
              {/* 切り替えスイッチと画面遷移ボタンを行で分け、狭い幅でも折り返しで崩れないようにする */}
              <div className="live-controls-toggles">
                <button onClick={handleToggle} className="live-control-btn" aria-label="接続切替">
                  <span className={`live-toggle-switch ${connected ? 'on' : ''}`}></span>
                  接続
                </button>
                <button onClick={goSettings} className="live-control-btn">設定</button>
              </div>
              <div className="live-controls-nav">
                <button onClick={goScenes} className="live-admin-btn">リモコン</button>
                <button onClick={goMemory} className="live-admin-btn">メモリ</button>
                <button onClick={goAdmin} className="live-admin-btn">ログ</button>
                <button onClick={goJobs} className="live-admin-btn">予約</button>
              </div>
            </div>
            <div className="live-character-area">
              <div
                className={`live-volume-control${isCharacterDim ? ' live-character-dim' : ''}`}
                aria-label="キャラクターエリア"
                role="button"
                aria-disabled={!isCharacterDim}
                onClick={() => void handleCharacterTap()}
              />
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
