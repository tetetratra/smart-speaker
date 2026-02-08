import React, { useCallback, useEffect, useRef, useState } from 'react'
import ReactDOM from 'react-dom/client'
import { createSpeechRecognizer, type SpeechHandle } from './speech'
import { downloadLanguageModel, startPresenceWatcher } from './vision'
import { createWS } from './ws'

type ChatMessage =
  | { id: number; type: 'user' | 'assistant' | 'system'; text: string; responseId?: string; final?: boolean; source?: string; expectation?: number }
  | { id: number; type: 'function_call'; toolCallId: string; name: string; args?: string }
  | { id: number; type: 'function_result'; toolCallId: string; output?: string }

const chatWSUrl = 'ws://localhost:8081/ws/chat'

function App() {
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [connected, setConnected] = useState(false)
  const [busy, setBusy] = useState(false)
  const [input, setInput] = useState('')
  const [rtcStatus, setRtcStatus] = useState('停止中')
  const [rtcError, setRtcError] = useState('')
  const [rtcAudioSeconds, setRtcAudioSeconds] = useState<number | null>(null)
  const [sttStatus, setSttStatus] = useState('停止中')
  const [sttError, setSttError] = useState('')
  const [sttInterim, setSttInterim] = useState('')
  const [presenceEnabled, setPresenceEnabled] = useState(false)
  const [presenceStatus, setPresenceStatus] = useState('停止中')
  const [presenceError, setPresenceError] = useState('')
  const [presencePresent, setPresencePresent] = useState<'yes' | 'no' | ''>('')
  const [presenceUpdatedAt, setPresenceUpdatedAt] = useState('')
  const [downloading, setDownloading] = useState(false)
  const idRef = useRef(0)
  const chatRef = useRef<HTMLDivElement | null>(null)
  const videoRef = useRef<HTMLVideoElement | null>(null)
  const audioRef = useRef<HTMLAudioElement | null>(null)

  const wsChatRef = useRef<ReturnType<typeof createWS> | null>(null)
  const stopPresenceRef = useRef<(() => void) | null>(null)
  const peerRef = useRef<RTCPeerConnection | null>(null)
  const micStreamRef = useRef<MediaStream | null>(null)
  const pendingICERef = useRef<RTCIceCandidateInit[]>([])
  const speechRef = useRef<SpeechHandle | null>(null)
  const statsTimerRef = useRef<number | null>(null)

  const nextMessageId = useCallback(() => {
    idRef.current += 1
    return idRef.current
  }, [])

  const appendMessage = useCallback((msg: ChatMessage) => {
    setMessages((prev) => [...prev, msg])
  }, [])

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

  const updateRTCStats = useCallback(async () => {
    const peer = peerRef.current
    if (!peer) return
    try {
      const stats = await peer.getStats()
      let inbound: any = null
      stats.forEach((report) => {
        if (report.type !== 'inbound-rtp') return
        const kind = (report as any).kind ?? (report as any).mediaType
        if (kind !== 'audio') return
        inbound = report
      })
      if (!inbound) return
      const inboundAny = inbound as any
      let seconds: number | null = null
      if (typeof inboundAny.totalSamplesDuration === 'number') {
        seconds = inboundAny.totalSamplesDuration
      } else if (typeof inboundAny.totalSamplesReceived === 'number') {
        let clockRate: number | null = null
        if (inboundAny.codecId) {
          const codec = stats.get(inboundAny.codecId)
          if (codec && typeof (codec as any).clockRate === 'number') {
            clockRate = (codec as any).clockRate
          }
        }
        if (clockRate && clockRate > 0) {
          seconds = inboundAny.totalSamplesReceived / clockRate
        }
      }
      if (seconds !== null && Number.isFinite(seconds)) {
        setRtcAudioSeconds(seconds)
      }
    } catch {
      return
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
          let role: 'user' | 'assistant' | 'system' = 'assistant'
          if (raw.role === 'user') role = 'user'
          else if (raw.role === 'system') role = 'system'
          const expectation = typeof raw.expectation === 'number' ? raw.expectation : undefined
          const displayText = raw.role ? text : `(roleなし) ${text}`
          appendMessage({
            id: nextMessageId(),
            type: role,
            text: displayText,
            responseId: raw.response_id,
            final: raw.final,
            source: typeof raw.source === 'string' ? raw.source : undefined,
            expectation,
          })
          break
        }
        case 'function_call': {
          appendMessage({
            id: nextMessageId(),
            type: 'function_call',
            toolCallId: String(raw.tool_call_id || ''),
            name: String(raw.name || ''),
            args: raw.arguments ? JSON.stringify(raw.arguments) : undefined,
          })
          break
        }
        case 'function_result': {
          appendMessage({
            id: nextMessageId(),
            type: 'function_result',
            toolCallId: String(raw.tool_call_id || ''),
            output: raw.output ? JSON.stringify(raw.output) : undefined,
          })
          break
        }
        default:
          break
      }
    },
    [appendMessage, handleRTCSignal, nextMessageId],
  )

  const sendSpeechText = useCallback(
    (text: string) => {
      const ws = wsChatRef.current
      const trimmed = text.trim()
      if (!ws || !trimmed) return
      ws.send({ type: 'message', role: 'user', text: trimmed })
      appendMessage({
        id: nextMessageId(),
        type: 'user',
        text: trimmed,
        final: true,
        source: 'browser-stt',
      })
    },
    [appendMessage, nextMessageId],
  )

  const sendSTTEvent = useCallback(
    (type: 'stt_start' | 'stt_end') => {
      const ws = wsChatRef.current
      if (!ws || !connected) return
      ws.send({
        type,
        source: 'browser-stt',
        captured_at: new Date().toISOString(),
      })
    },
    [connected],
  )

  useEffect(() => {
    const speech = createSpeechRecognizer({
      onFinal: (text) => {
        sendSpeechText(text)
        setSttInterim('')
      },
      onInterim: (text) => setSttInterim(text),
      onStart: () => {
        setSttStatus('認識中')
        setSttError('')
        sendSTTEvent('stt_start')
      },
      onSpeechEnd: () => {
        sendSTTEvent('stt_end')
      },
      onEnd: () => setSttStatus('停止中'),
      onError: (message) => {
        setSttStatus('エラー')
        setSttError(message)
      },
    })
    speechRef.current = speech
    if (!speech.isSupported) {
      setSttStatus('未対応')
    }
    return () => {
      speech.abort()
      speechRef.current = null
    }
  }, [sendSpeechText, sendSTTEvent])

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
    speechRef.current?.stop()
    setSttInterim('')
    pendingICERef.current = []
    if (statsTimerRef.current !== null) {
      window.clearInterval(statsTimerRef.current)
      statsTimerRef.current = null
    }
    setRtcAudioSeconds(null)
    setRtcStatus('停止中')
    setRtcError('')
  }, [])

  const startRTC = useCallback(async () => {
    if (peerRef.current) return
    setRtcError('')
    const ws = wsChatRef.current
    if (!ws) return

    const peer = new RTCPeerConnection()
    peerRef.current = peer
    peer.addTransceiver('audio', { direction: 'recvonly' })
    if (statsTimerRef.current !== null) {
      window.clearInterval(statsTimerRef.current)
      statsTimerRef.current = null
    }
    statsTimerRef.current = window.setInterval(() => {
      updateRTCStats()
    }, 1000)

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
      if (audioRef.current) {
        audioRef.current.srcObject = stream
        audioRef.current.play().catch(() => {})
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
      stopRTC()
      return
    }
    micStreamRef.current = stream
    const audioTrack = stream.getAudioTracks()[0]
    if (audioTrack && speechRef.current?.isSupported) {
      speechRef.current.start(audioTrack)
    } else if (!audioTrack) {
      setSttStatus('エラー')
      setSttError('マイクトラックを取得できませんでした')
    }

    try {
      const offer = await peer.createOffer()
      await peer.setLocalDescription(offer)
      ws.send({ type: 'webrtc.offer', sdp: offer.sdp })
      setRtcStatus('接続中')
    } catch (err) {
      setRtcError(err instanceof Error ? err.message : 'RTC start error')
      stopRTC()
    }
  }, [stopRTC, updateRTCStats])

  const connect = useCallback(async () => {
    if (connected || busy) return
    setBusy(true)
    try {
      const wsChat = createWS(chatWSUrl)
      await wsChat.connect((msg) => {
        handleChatMessage(msg)
      })

      wsChatRef.current = wsChat
      setConnected(true)
      await startRTC()
      appendMessage({ id: Date.now(), type: 'assistant', text: '接続しました。話しかけてください。' })
    } catch (e) {
      console.error('connect error', e)
      stopRTC()
      setConnected(false)
      wsChatRef.current?.close()
      wsChatRef.current = null
      throw e
    } finally {
      setBusy(false)
    }
  }, [appendMessage, busy, connected, handleChatMessage, startRTC, stopRTC])

  const disconnect = useCallback(() => {
    stopRTC()
    wsChatRef.current?.close()
    wsChatRef.current = null
    setConnected(false)
  }, [stopRTC])

  useEffect(() => {
    return () => {
      disconnect()
      stopPresenceRef.current?.()
      stopPresenceRef.current = null
    }
  }, [disconnect])

  const sendPresence = useCallback(
    (present: 'yes' | 'no', capturedAt: string) => {
      const ws = wsChatRef.current
      if (!ws || !connected) return
      ws.send({
        type: 'presence',
        present,
        captured_at: capturedAt,
      })
    },
    [connected],
  )

  const startPresence = useCallback(async () => {
    if (presenceEnabled) {
      stopPresenceRef.current?.()
      stopPresenceRef.current = null
      setPresenceEnabled(false)
      setPresenceStatus('停止中')
      setPresenceError('')
      setPresencePresent('')
      setPresenceUpdatedAt('')
      sendPresence('no', new Date().toISOString())
      return
    }
    if (!videoRef.current) {
      return
    }
    setPresenceEnabled(true)
    setPresenceError('')
    try {
      const handle = await startPresenceWatcher({
        video: videoRef.current,
        intervalMs: 10000,
        onStatus: (status, message) => {
          if (status === 'error') {
            setPresenceStatus('エラー')
            setPresenceError(message ?? '')
            return
          }
          if (status === 'unsupported') {
            setPresenceStatus('未対応')
            setPresenceError(message ?? '')
            return
          }
          if (status === 'starting') {
            setPresenceStatus(message ? `起動中: ${message}` : '起動中')
            return
          }
          if (status === 'ready') {
            setPresenceStatus(message ? `待機中: ${message}` : '待機中')
            return
          }
          if (status === 'running') {
            setPresenceStatus(message ? `解析中: ${message}` : '解析中')
            return
          }
          if (status === 'idle') {
            setPresenceStatus('停止中')
          }
        },
        onResult: (result) => {
          setPresencePresent(result.present)
          const time = new Date(result.capturedAt).toLocaleTimeString('ja-JP', { hour12: false })
          setPresenceUpdatedAt(time)
          sendPresence(result.present, result.capturedAt)
        },
      })
      stopPresenceRef.current = handle.stop
    } catch (err) {
      setPresenceEnabled(false)
      setPresenceStatus('エラー')
      setPresenceError(err instanceof Error ? err.message : 'unknown error')
    }
  }, [presenceEnabled, sendPresence])

  const downloadModel = useCallback(async () => {
    if (downloading) return
    setDownloading(true)
    setPresenceError('')
    try {
      await downloadLanguageModel((status, message) => {
        if (status === 'error') {
          setPresenceStatus('エラー')
          setPresenceError(message ?? '')
          return
        }
        if (status === 'unsupported') {
          setPresenceStatus('未対応')
          setPresenceError(message ?? '')
          return
        }
        if (status === 'starting') {
          setPresenceStatus(message ? `起動中: ${message}` : '起動中')
          return
        }
        if (status === 'ready') {
          setPresenceStatus(message ? `待機中: ${message}` : '待機中')
          return
        }
        if (status === 'running') {
          setPresenceStatus(message ? `解析中: ${message}` : '解析中')
          return
        }
      })
      setPresenceStatus('待機中')
    } catch (err) {
      setPresenceStatus('エラー')
      setPresenceError(err instanceof Error ? err.message : 'unknown error')
    } finally {
      setDownloading(false)
    }
  }, [downloading])

  useEffect(() => {
    const el = chatRef.current
    if (el) {
      el.scrollTop = el.scrollHeight
    }
  }, [messages])

  const sendReset = useCallback(() => {
    const ws = wsChatRef.current
    if (!ws || !connected) return
    ws.send({ type: 'reset' })
  }, [connected])

  const sendText = useCallback(() => {
    const ws = wsChatRef.current
    const text = input.trim()
    if (!ws || !connected || !text) return
    const msg = { type: 'message', role: 'user', text }
    ws.send(msg)
    setMessages((prev) => [
      ...prev,
      { id: Date.now(), type: 'user', text, responseId: undefined, final: true },
    ])
    setInput('')
  }, [connected, input])

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
      <h1>WS Audio Client</h1>
      <div style={{ display: 'flex', gap: 8 }}>
        <button onClick={connect} disabled={connected || busy}>
          接続
        </button>
        <button onClick={disconnect} disabled={!connected}>
          切断
        </button>
        <button onClick={startPresence} disabled={busy}>
          {presenceEnabled ? 'カメラ停止' : 'カメラ開始'}
        </button>
        <button onClick={downloadModel} disabled={downloading}>
          {downloading ? 'モデルDL中' : 'モデルDL開始'}
        </button>
        <button onClick={sendReset} disabled={!connected}>
          おやすみ
        </button>
      </div>
      <div style={{ display: 'flex', gap: 16, alignItems: 'flex-start' }}>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          <div>
            <strong>カメラ状態:</strong> {presenceStatus}
          </div>
          {presenceError && (
            <div style={{ color: '#dc2626' }}>
              <strong>エラー:</strong> {presenceError}
            </div>
          )}
          <video ref={videoRef} width={160} height={120} autoPlay muted style={{ transform: 'scaleX(-1)' }} />
        </div>
        <div style={{ flex: 1 }}>
          <div style={{ marginBottom: 8 }}>
            <strong>人の有無:</strong> {presencePresent || '（なし）'}
          </div>
          <div>
            <strong>撮影時刻:</strong> {presenceUpdatedAt || '（なし）'}
          </div>
        </div>
      </div>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
        <div>
          <strong>音声接続:</strong> {rtcStatus}
        </div>
        {rtcError && (
          <div style={{ color: '#dc2626' }}>
            <strong>音声エラー:</strong> {rtcError}
          </div>
        )}
        <div>
          <strong>受信音声(合計):</strong>{' '}
          {rtcAudioSeconds === null ? '（取得不可）' : `${rtcAudioSeconds.toFixed(2)}秒`}
        </div>
        <div>
          <strong>文字起こし:</strong> {sttStatus}
        </div>
        {sttInterim && (
          <div style={{ color: '#0f766e' }}>
            <strong>認識中:</strong> {sttInterim}
          </div>
        )}
        {sttError && (
          <div style={{ color: '#dc2626' }}>
            <strong>文字起こしエラー:</strong> {sttError}
          </div>
        )}
      </div>
      <audio ref={audioRef} autoPlay />
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
          if (m.type === 'function_call') {
            return (
              <div key={m.id} style={{ marginBottom: 8 }}>
                <strong style={{ color: '#8b5cf6' }}>function call</strong>
                <div>name: {m.name}</div>
                <div>callId: {m.toolCallId}</div>
                {m.args && <div>args: {m.args}</div>}
              </div>
            )
          }
          if (m.type === 'function_result') {
            return (
              <div key={m.id} style={{ marginBottom: 8 }}>
                <strong style={{ color: '#ec4899' }}>function result</strong>
                <div>callId: {m.toolCallId}</div>
                {m.output && <div>output: {m.output}</div>}
              </div>
            )
          }
          let color = '#16a34a'
          let label = 'Assistant'
          if (m.type === 'user') {
            color = '#2563eb'
            label = 'User'
          } else if (m.type === 'system') {
            color = '#6b7280'
            label = 'System'
          }
          const sourceLabel =
            m.source === 'conversation-chain' ? ' (chain)' : m.source ? ` (${m.source})` : ''
          return (
            <div key={m.id} style={{ marginBottom: 8 }}>
              <strong style={{ color }}>{label}{sourceLabel}</strong>
              {typeof m.expectation === 'number' && (
                <span style={{ marginLeft: 8, color: '#0f766e', fontSize: 12 }}>
                  期待度: {m.expectation}
                </span>
              )}
              <div>{m.text}</div>
            </div>
          )
        })}
      </div>
      <div style={{ display: 'flex', gap: 8 }}>
        <input
          type="text"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          placeholder="テキストで話しかける"
          style={{ flex: 1, padding: 8, borderRadius: 6, border: '1px solid #ddd' }}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && !e.nativeEvent.isComposing) {
              e.preventDefault()
              sendText()
            }
          }}
        />
        <button onClick={sendText} disabled={!connected || !input.trim()}>
          送信
        </button>
      </div>
    </div>
  )
}

const rootEl = document.getElementById('root')
if (!rootEl) {
  throw new Error('root element not found')
}
ReactDOM.createRoot(rootEl).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
)
