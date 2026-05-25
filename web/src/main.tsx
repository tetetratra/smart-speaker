import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import ReactDOM from 'react-dom/client'
import { createWS } from './ws'

type ChatMessage =
  | { id: number; type: 'user' | 'agent' | 'tool_call' | 'tool_result' | 'system'; text: string; responseId?: string; final?: boolean; source?: string }

type StatusTone = 'idle' | 'active' | 'done' | 'error'
type ButtonTone = 'primary' | 'secondary'
type BoardEntry = { id: number; content: string }
type ToolToastKind = 'call' | 'result'
type ToolToast = { id: number; kind: ToolToastKind; toolName: string }

const browserURL = new URL(window.location.href)
const backendURL = new URL(window.location.origin)
if (browserURL.port === '5173') {
  backendURL.port = '8081'
}
const wsProtocol = backendURL.protocol === 'https:' ? 'wss' : 'ws'
const chatWSUrl = `${wsProtocol}://${backendURL.host}/ws/chat`
const serverHTTPBaseUrl = backendURL.origin
const reconnectMaxAttempts = 10
const reconnectInitialDelayMs = 1000
const nightModeStartHour = 22
const nightModeEndHour = 6
const minuteMs = 60 * 1000
const toolToastDurationMs = 5000

type PlaybackVolumeLevel = 'quiet' | 'low' | 'normal' | 'boost'

const playbackVolumePresets: Record<PlaybackVolumeLevel, { label: string; gain: number }> = {
  quiet: { label: '0.3倍', gain: 0.3 },
  low: { label: '0.6倍', gain: 0.6 },
  normal: { label: '1倍', gain: 1.0 },
  boost: { label: '1.5倍', gain: 1.5 },
}
const playbackVolumeLevels: PlaybackVolumeLevel[] = ['quiet', 'low', 'normal', 'boost']
const defaultPlaybackVolumeLevel: PlaybackVolumeLevel = 'low'
const liveRootStyle = `
  :root {
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
    padding: 6px;
    position: relative;
    background: var(--live-bg);
    color: var(--live-text);
  }
  .live-frame.live-night-mode {
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
    grid-template-rows: auto auto auto 1fr;
    gap: 8px;
  }
  .live-controls-row {
    display: flex;
    justify-content: space-between;
    gap: 6px;
    width: 100%;
    align-items: center;
  }
  .live-controls-actions {
    display: flex;
    justify-content: flex-end;
    gap: 6px;
    align-items: center;
    flex: 0 0 auto;
  }
  .live-volume-control {
    border: 1px solid var(--live-line);
    background: var(--live-panel-soft);
    border-radius: 10px;
    padding: 8px 10px 7px;
    position: relative;
    overflow: hidden;
    min-height: 88px;
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
    background: #fff7ed;
    border-color: #fdba74;
    color: #9a3412;
  }
  .live-tool-toast.result {
    background: #eff6ff;
    border-color: #93c5fd;
    color: #1d4ed8;
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
    accent-color: var(--live-toggle-on);
    cursor: pointer;
    margin: 0;
  }
  .live-audio-stats {
    display: flex;
    gap: 4px;
    align-items: center;
    min-width: 0;
    flex-wrap: wrap;
  }
  .live-audio-stat {
    border: 1px solid var(--live-line);
    background: var(--live-panel-soft);
    border-radius: 999px;
    padding: 4px 8px;
    font-size: 10px;
    line-height: 1;
    color: var(--live-muted);
    display: inline-flex;
    align-items: center;
    gap: 4px;
    white-space: nowrap;
  }
  .live-audio-stat strong {
    color: var(--live-text);
    font-weight: 700;
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
  .live-bubble {
    background: var(--live-panel-raised);
    border: 1px solid var(--live-line);
    border-radius: 14px;
    padding: 12px 14px;
    font-size: 24px;
    line-height: 1.3;
    position: relative;
    min-height: 120px;
    box-shadow: 0 1px 2px var(--live-shadow);
    text-align: center;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .live-bubble-content {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 10px;
    width: 100%;
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
  .live-mini {
    width: 100%;
    height: 100%;
    border-radius: 14px;
    border: 1px dashed var(--live-line);
    background: var(--live-panel-soft);
    color: var(--live-muted);
    display: grid;
    place-items: center;
    font-size: 11px;
    min-height: 0;
  }
`

function getStatusTone(status: string): StatusTone {
  if (status.includes('失敗') || status.includes('エラー')) return 'error'
  if (status === '接続済み' || status === '完了') return 'done'
  if (status === '接続中' || status === '送信中' || status === '検知中' || status === '認識中' || status === '最終結果待ち') {
    return 'active'
  }
  return 'idle'
}

function getStatusBadgeStyle(tone: StatusTone): React.CSSProperties {
  switch (tone) {
    case 'active':
      return { background: '#fff7ed', color: '#c2410c', border: '1px solid #fdba74' }
    case 'done':
      return { background: '#f0fdf4', color: '#166534', border: '1px solid #86efac' }
    case 'error':
      return { background: '#fef2f2', color: '#b91c1c', border: '1px solid #fca5a5' }
    default:
      return { background: '#f8fafc', color: '#475569', border: '1px solid #cbd5e1' }
  }
}

function getButtonStyle(tone: ButtonTone, disabled: boolean): React.CSSProperties {
  const base: React.CSSProperties = {
    borderRadius: 12,
    padding: '10px 14px',
    fontSize: 14,
    fontWeight: 700,
    border: '1px solid transparent',
    transition: 'background-color 120ms ease, border-color 120ms ease, color 120ms ease, opacity 120ms ease',
    cursor: disabled ? 'not-allowed' : 'pointer',
    opacity: disabled ? 0.45 : 1,
  }
  if (tone === 'primary') {
    return {
      ...base,
      background: '#0f172a',
      color: '#f8fafc',
      borderColor: '#0f172a',
    }
  }
  return {
    ...base,
    background: '#ffffff',
    color: '#334155',
    borderColor: '#cbd5e1',
  }
}

function isNightModeTime(date: Date): boolean {
  const hour = date.getHours()
  return hour >= nightModeStartHour || hour < nightModeEndHour
}

function getPipelineStateOptions(label: string): string[] {
  switch (label) {
    case 'WebRTC接続':
      return ['停止中', '接続中', '接続済み', '切断', '失敗']
    case 'マイクストリーム送信':
      return ['停止中', '確認中', '待機中', '送信中', '確認失敗']
    case 'サーバー発話検知':
      return ['待機中', '検知中']
    case 'Google文字起こし':
      return ['停止中', '待機中', '認識中', '最終結果待ち', '完了', 'エラー']
    default:
      return []
  }
}

type LiveViewProps = {
  connected: boolean
  connecting: boolean
  speechDetectStatus: string
  sttStatus: string
  inputLevel: number
  speechThreshold: number
  lastAssistantMessage: string
  lastUserMessage: string
  boardEntries: BoardEntry[]
  toolToasts: ToolToast[]
  playbackVolumeLevel: PlaybackVolumeLevel
  onPlaybackVolumeChange: (level: PlaybackVolumeLevel) => void
  connect: () => Promise<void>
  disconnect: () => void
  goAdmin: () => void
}

function App() {
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [connected, setConnected] = useState(false)
  const [busy, setBusy] = useState(false)
  const [rtcStatus, setRtcStatus] = useState('停止中')
  const [rtcError, setRtcError] = useState('')
  const [audioSendStatus, setAudioSendStatus] = useState('停止中')
  const [speechDetectStatus, setSpeechDetectStatus] = useState('待機中')
  const [sttStatus, setSttStatus] = useState('停止中')
  const [sttError, setSttError] = useState('')
  const [playbackVolumeLevel, setPlaybackVolumeLevel] = useState<PlaybackVolumeLevel>(defaultPlaybackVolumeLevel)
  const [inputLevel, setInputLevel] = useState(0)
  const [speechThreshold, setSpeechThreshold] = useState(0)
  const [boardEntries, setBoardEntries] = useState<BoardEntry[]>([])
  const [toolToasts, setToolToasts] = useState<ToolToast[]>([])
  const idRef = useRef(0)
  const chatRef = useRef<HTMLDivElement | null>(null)
  const remoteStreamRef = useRef<MediaStream | null>(null)
  const audioContextRef = useRef<AudioContext | null>(null)
  const remoteSourceRef = useRef<MediaStreamAudioSourceNode | null>(null)
  const playbackGainRef = useRef<GainNode | null>(null)

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
    if (remoteSourceRef.current) {
      remoteSourceRef.current.disconnect()
      remoteSourceRef.current = null
    }
    if (playbackGainRef.current) {
      playbackGainRef.current.disconnect()
      playbackGainRef.current = null
    }
    if (audioContextRef.current) {
      void audioContextRef.current.close()
      audioContextRef.current = null
    }
  }, [])

  const connectRemoteStreamToAudioGraph = useCallback(async () => {
    const stream = remoteStreamRef.current
    if (!stream) return

    stopRemoteAudioGraph()
    const AudioContextClass = window.AudioContext || (window as typeof window & { webkitAudioContext?: typeof AudioContext }).webkitAudioContext
    if (!AudioContextClass) {
      throw new Error('Web Audio APIを利用できません')
    }

    const audioContext = new AudioContextClass()
    const source = audioContext.createMediaStreamSource(stream)
    const gain = audioContext.createGain()
    gain.gain.value = selectedPlaybackVolume.gain
    source.connect(gain)
    gain.connect(audioContext.destination)

    audioContextRef.current = audioContext
    remoteSourceRef.current = source
    playbackGainRef.current = gain
    if (audioContext.state === 'suspended') {
      await audioContext.resume()
    }
  }, [selectedPlaybackVolume.gain, stopRemoteAudioGraph])

  useEffect(() => {
    if (!playbackGainRef.current) return
    playbackGainRef.current.gain.value = selectedPlaybackVolume.gain
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
      if (!raw || typeof raw !== 'object') return
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
          if (raw.source === 'server-stt' && role === 'user') {
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
          })
          break
        }
        case 'speech_end': {
          setSpeechDetectStatus('待機中')
          setSttStatus('最終結果待ち')
          break
        }
        case 'rtc_vad_status': {
          const nextInputLevel = typeof raw.input_level === 'number' ? Math.max(0, Math.round(raw.input_level)) : 0
          const nextThreshold = typeof raw.threshold === 'number' ? Math.max(0, Math.round(raw.threshold)) : 0
          setInputLevel(nextInputLevel)
          setSpeechThreshold(nextThreshold)
          break
        }
        case 'whiteboard_update': {
          const content = typeof raw.content === 'string' ? raw.content.trim() : ''
          if (!content) return
          setBoardEntries((prev) => [...prev, { id: nextMessageId(), content }])
          break
        }
        default:
          break
      }
    },
    [appendMessage, handleRTCSignal, nextMessageId, showToolToast],
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
    remoteStreamRef.current = null
    stopRemoteAudioGraph()
    pendingICERef.current = []
    lastAudioBytesSentRef.current = null
    setRtcStatus('停止中')
    setAudioSendStatus('停止中')
    setSpeechDetectStatus('待機中')
    setRtcError('')
    setSttStatus('停止中')
    setInputLevel(0)
    setSpeechThreshold(0)
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
    peer.ontrack = (event) => {
      const stream = event.streams?.[0] ?? new MediaStream([event.track])
      remoteStreamRef.current = stream
      connectRemoteStreamToAudioGraph().catch((err) => {
        setRtcStatus('失敗')
        setRtcError(err instanceof Error ? err.message : 'audio playback error')
        stopRTC()
      })
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
  }, [connectRemoteStreamToAudioGraph, stopRTC])

  const openConnection = useCallback(async (isAutoReconnect: boolean) => {
    if (connected || busy) return
    setBusy(true)
    try {
      const wsChat = createWS(chatWSUrl)
      await wsChat.connect((msg) => {
        handleChatMessage(msg)
      }, () => {
        stopRTC()
        setConnected(false)
        if (wsChatRef.current === wsChat) {
          wsChatRef.current = null
        }
        scheduleReconnect()
      })

      wsChatRef.current = wsChat
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
  }, [appendMessage, busy, clearReconnectTimer, connected, handleChatMessage, scheduleReconnect, startRTC, stopRTC])

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
  }, [clearReconnectTimer, stopRTC])

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
          if (typeof (report as RTCOutboundRtpStreamStats).bytesSent !== 'number') return
          currentBytesSent = (report as RTCOutboundRtpStreamStats).bytesSent
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
    const el = chatRef.current
    if (el) {
      el.scrollTop = el.scrollHeight
    }
  }, [messages])
  const startGoogleAuth = useCallback(() => {
    const url = `${serverHTTPBaseUrl}/oauth/google/start`
    const opened = window.open(url, '_blank')
    if (!opened) {
      window.location.href = url
    }
  }, [])

  const pipelineStatuses = [
    { label: 'WebRTC接続', status: rtcStatus },
    { label: 'マイクストリーム送信', status: audioSendStatus },
    { label: 'サーバー発話検知', status: speechDetectStatus },
    { label: 'Google文字起こし', status: sttStatus },
  ]
  const lastAssistantMessage = useMemo(() => {
    for (let i = messages.length - 1; i >= 0; i -= 1) {
      const msg = messages[i]
      if (msg.type === 'agent') return msg.text
    }
    return ''
  }, [messages])
  const lastUserMessage = useMemo(() => {
    for (let i = messages.length - 1; i >= 0; i -= 1) {
      const msg = messages[i]
      if (msg.type === 'user') return msg.text
    }
    return ''
  }, [messages])
  const [uiMode, setUiMode] = useState<'admin' | 'app'>(() => {
    const params = new URLSearchParams(window.location.search)
    if (params.get('ui') === 'admin') return 'admin'
    if (window.location.pathname.startsWith('/admin')) return 'admin'
    return 'app'
  })
  useEffect(() => {
    const onPopState = () => {
      const params = new URLSearchParams(window.location.search)
      if (params.get('ui') === 'admin') {
        setUiMode('admin')
        return
      }
      if (window.location.pathname.startsWith('/admin')) {
        setUiMode('admin')
        return
      }
      setUiMode('app')
    }
    window.addEventListener('popstate', onPopState)
    return () => window.removeEventListener('popstate', onPopState)
  }, [])
  useEffect(() => {
    const body = document.body
    if (uiMode === 'admin') {
      body.classList.add('admin-mode')
    } else {
      body.classList.remove('admin-mode')
    }
    return () => body.classList.remove('admin-mode')
  }, [uiMode])
  const setMode = useCallback((mode: 'admin' | 'app') => {
    const params = new URLSearchParams(window.location.search)
    if (mode === 'admin') {
      params.set('ui', 'admin')
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
          inputLevel={inputLevel}
          speechThreshold={speechThreshold}
          lastAssistantMessage={lastAssistantMessage}
          lastUserMessage={lastUserMessage}
          boardEntries={boardEntries}
          toolToasts={toolToasts}
          playbackVolumeLevel={playbackVolumeLevel}
          onPlaybackVolumeChange={setPlaybackVolumeLevel}
          connect={connect}
          disconnect={disconnect}
          goAdmin={() => setMode('admin')}
        />
      </div>
      <div
        style={{
          display: uiMode === 'admin' ? 'flex' : 'none',
          flexDirection: 'column',
          gap: 12,
          padding: 12,
          minHeight: '100vh',
          boxSizing: 'border-box',
        }}
      >
        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
          <button onClick={connect} disabled={connected || busy} style={getButtonStyle('primary', connected || busy)}>
            接続
          </button>
          <button onClick={disconnect} disabled={!connected} style={getButtonStyle('secondary', !connected)}>
            切断
          </button>
          <button onClick={startGoogleAuth} style={getButtonStyle('secondary', false)}>
            Google認証
          </button>
          <button onClick={() => setMode('app')} style={getButtonStyle('secondary', false)}>
            アプリ画面へ
          </button>
        </div>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))', gap: 8 }}>
            {pipelineStatuses.map((item) => {
              const tone = getStatusTone(item.status)
              const options = getPipelineStateOptions(item.label)
              return (
                <div
                  key={item.label}
                  style={{
                    borderRadius: 10,
                    border: '1px solid #e2e8f0',
                    background: '#ffffff',
                    padding: 10,
                    display: 'flex',
                    flexDirection: 'column',
                    gap: 8,
                  }}
                >
                  <div style={{ fontSize: 12, color: '#475569' }}>{item.label}</div>
                  <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
                    {options.map((option) => {
                      const isCurrent = option === item.status
                      return (
                        <span
                          key={option}
                          style={{
                            ...(isCurrent
                              ? getStatusBadgeStyle(tone)
                              : {
                                  background: '#f8fafc',
                                  color: '#94a3b8',
                                  border: '1px solid #e2e8f0',
                                }),
                            display: 'inline-flex',
                            alignItems: 'center',
                            borderRadius: 9999,
                            padding: '4px 10px',
                            fontSize: 12,
                            fontWeight: isCurrent ? 700 : 500,
                            opacity: isCurrent ? 1 : 0.55,
                          }}
                        >
                          {option}
                        </span>
                      )
                    })}
                  </div>
                </div>
              )
            })}
          </div>
          {rtcError && (
            <div style={{ color: '#dc2626' }}>
              <strong>音声エラー:</strong> {rtcError}
            </div>
          )}
          <div>
            <strong>再生音量:</strong> {selectedPlaybackVolume.label} (gain {selectedPlaybackVolume.gain})
          </div>
          {sttError && (
            <div style={{ color: '#dc2626' }}>
              <strong>文字起こしエラー:</strong> {sttError}
            </div>
          )}
        </div>
        <div
          style={{
            border: '1px solid #ddd',
            borderRadius: 8,
            padding: 12,
            minHeight: 300,
            maxHeight: 500,
            overflowY: 'auto',
            background: '#fafafa',
          }}
          ref={chatRef}
        >
          {messages.map((m) => {
            let color = '#16a34a'
            let label = 'Agent'
            if (m.type === 'user') {
              color = '#2563eb'
              label = 'User'
            } else if (m.type === 'system') {
              color = '#6b7280'
              label = 'System'
            } else if (m.type === 'tool_call') {
              color = '#8b5cf6'
              label = 'Tool call'
            } else if (m.type === 'tool_result') {
              color = '#ec4899'
              label = 'Tool result'
            }
            const sourceLabel = m.source ? ` (${m.source})` : ''
            return (
              <div key={m.id} style={{ marginBottom: 8 }}>
                <strong style={{ color }}>{label}{sourceLabel}</strong>
                <div>{m.text}</div>
              </div>
            )
          })}
        </div>
      </div>
    </>
  )
}

function LiveView(props: LiveViewProps) {
  const {
    connected,
    connecting,
    speechDetectStatus,
    sttStatus,
    inputLevel,
    speechThreshold,
    lastAssistantMessage,
    lastUserMessage,
    boardEntries,
    toolToasts,
    playbackVolumeLevel,
    onPlaybackVolumeChange,
    connect,
    disconnect,
    goAdmin,
  } = props
  const [isNightMode, setIsNightMode] = useState(() => isNightModeTime(new Date()))
  const boardRef = useRef<HTMLDivElement | null>(null)
  const handleToggle = useCallback(() => {
    if (connected) {
      disconnect()
      return
    }
    void connect()
  }, [connected, connect, disconnect])
  useEffect(() => {
    const updateNightMode = () => setIsNightMode(isNightModeTime(new Date()))
    updateNightMode()
    const timer = window.setInterval(updateNightMode, minuteMs)
    return () => window.clearInterval(timer)
  }, [])
  useEffect(() => {
    const board = boardRef.current
    if (!board) return
    board.scrollTop = board.scrollHeight
  }, [boardEntries])
  const connectionStatus = connecting ? '接続中' : connected ? 'オンライン' : 'オフライン'
  const playbackVolumeIndex = playbackVolumeLevels.indexOf(playbackVolumeLevel)
  const selectedPlaybackVolume = playbackVolumePresets[playbackVolumeLevel]
  return (
    <>
      <style>{liveRootStyle}</style>
      <div className={`live-frame ${isNightMode ? 'live-night-mode' : ''}`}>
        <div className="live-main">
          <div className="live-left">
            <div className="live-board" ref={boardRef}>
              {boardEntries.length === 0 ? (
                <div className="live-board-empty">ホワイトボード</div>
              ) : (
                boardEntries.map((entry, index) => (
                  <React.Fragment key={entry.id}>
                    {index > 0 && <hr className="live-board-separator" />}
                    <div className="live-board-entry">
                      {entry.content}
                    </div>
                  </React.Fragment>
                ))
              )}
            </div>
            <div className="live-bubble">
              {lastAssistantMessage && (
                <div className="live-bubble-content">
                  {lastUserMessage && <div className="live-bubble-user">{lastUserMessage}</div>}
                  <div className="live-bubble-agent">{lastAssistantMessage}</div>
                </div>
              )}
            </div>
          </div>
          <div className="live-right">
            <div className="live-controls-row">
              <div className="live-audio-stats" aria-label="VAD状態">
                <div className="live-audio-stat">音量 <strong>{inputLevel}</strong></div>
                <div className="live-audio-stat">しきい値 <strong>{speechThreshold}</strong></div>
              </div>
              <div className="live-controls-actions">
                <button onClick={handleToggle} className="live-control-btn" aria-label="接続切替">
                  <span className={`live-toggle-switch ${connected ? 'on' : ''}`}></span>
                  接続
                </button>
                <button onClick={goAdmin} className="live-admin-btn">管理画面</button>
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
            <div className="live-volume-control" aria-label="キャラクターエリア">
              <div className="live-tool-toast-stack">
                {toolToasts.map((toast) => (
                  <div key={toast.id} className={`live-tool-toast ${toast.kind}`}>
                    <span className="live-tool-toast-tool">{toast.kind === 'call' ? 'tool call' : 'tool result'}: {toast.toolName}</span>
                  </div>
                ))}
              </div>
            </div>
            <div className="live-mini"></div>
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
