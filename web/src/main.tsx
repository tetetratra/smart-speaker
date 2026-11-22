// テーマを先に読み込み、各コンポーネントの CSS がその後に来るようにする。
import './shared/theme.css'
import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import ReactDOM from 'react-dom/client'
import { createWS } from './ws'
import { conversationBubble } from './conversation'
import { Whiteboard, type HistoryEntry } from './Whiteboard'
import { historyEntries, mergeRecords, splitRecords, type WhiteboardRecord } from './whiteboardHistory'
import { useWakeMode, type WakeMode } from './features/wakemode/useWakeMode'
import { SettingsView } from './SettingsView'
import { AutomationScreen } from './features/automations/AutomationScreen'
import { MemoryScreen } from './features/memory/MemoryScreen'
import { EventLogScreen } from './features/eventlog/EventLogScreen'
import { useEventLog } from './features/eventlog/useEventLog'
import { defaultPlaybackVolumeLevel, playbackVolumeGains, usePlaybackVolume } from './plugins/playbackvolume/usePlaybackVolume'
import { SceneScreen } from './plugins/switchbot/SceneScreen'
import { createAudioReceiver, type AudioReceiver, type PlaybackSpeech } from './audio'
import { startBubbleAutoScroll } from './bubbleScroll'
import { wsURL } from './shared/backend'
import { useScreen } from './app/routes'

// ChatMessage はチャット欄（吹き出し）に出すユーザーの発話。AI の発話は playback_progress から作る。
type ChatMessage = { id: number; type: 'user'; text: string; generationId?: number }

const chatWSUrl = wsURL('/ws/chat')
const reconnectMaxAttempts = 10
const reconnectInitialDelayMs = 1000

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
  const [serverEventLogs, appendServerEventLog] = useEventLog()
  const [isAiSpeaking, setIsAiSpeaking] = useState(false)
  const currentGenerationRef = useRef(0)
  const idRef = useRef(0)
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


  const bubble = useMemo(() => conversationBubble(messages, playbackSpeech), [messages, playbackSpeech])
  const lastAssistantMessage = bubble.assistant
  const lastUserMessage = bubble.user
  const [screen, navigate] = useScreen()
  const goLive = useCallback(() => navigate('live'), [navigate])

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
      {screen === 'eventlog' && <EventLogScreen lines={serverEventLogs} onBack={goLive} />}
      {screen === 'jobs' && <AutomationScreen onBack={goLive} />}
      {screen === 'memory' && <MemoryScreen onBack={goLive} />}
      {screen === 'scenes' && <SceneScreen onBack={goLive} />}
      {screen === 'settings' && <SettingsView
        connectionStatus={connectionStatusLabel(connected, busy)}
        speechDetectStatus={speechDetectStatus}
        sttStatus={sttStatus}
        wakeMode={wakeMode}
        playbackVolumeLevel={playbackVolumeLevel}
        onPlaybackVolumeChange={setPlaybackVolumeLevel}
        onBack={goLive}
      />}
    </>
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
