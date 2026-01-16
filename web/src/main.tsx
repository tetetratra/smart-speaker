import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import ReactDOM from 'react-dom/client'
import { createAudioReceiver } from './audio'
import { createSpeechRecognizer } from './speech'
import { downloadLanguageModel, startPresenceWatcher } from './vision'
import { createWS } from './ws'

type ChatMessage =
  | { id: number; type: 'user' | 'assistant' | 'system'; text: string; responseId?: string; final?: boolean; source?: string }
  | { id: number; type: 'function_call'; toolCallId: string; name: string; args?: string }
  | { id: number; type: 'function_result'; toolCallId: string; output?: string }

const audioWSUrl = 'ws://localhost:8081/ws/audio'
const chatWSUrl = 'ws://localhost:8081/ws/chat'
const silenceTimeoutMs = 1200

function App() {
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [connected, setConnected] = useState(false)
  const [busy, setBusy] = useState(false)
  const [input, setInput] = useState('')
  const [voiceStatus, setVoiceStatus] = useState('停止中')
  const [speechError, setSpeechError] = useState('')
  const [transcriptFinal, setTranscriptFinal] = useState('')
  const [transcriptInterim, setTranscriptInterim] = useState('')
  const [presenceEnabled, setPresenceEnabled] = useState(false)
  const [presenceStatus, setPresenceStatus] = useState('停止中')
  const [presenceError, setPresenceError] = useState('')
  const [presencePresent, setPresencePresent] = useState<'yes' | 'no' | ''>('')
  const [presenceUpdatedAt, setPresenceUpdatedAt] = useState('')
  const [downloading, setDownloading] = useState(false)
  const idRef = useRef(0)
  const chatRef = useRef<HTMLDivElement | null>(null)
  const videoRef = useRef<HTMLVideoElement | null>(null)

  const receiver = useMemo(() => createAudioReceiver(), [])

  const wsAudioRef = useRef<ReturnType<typeof createWS> | null>(null)
  const wsChatRef = useRef<ReturnType<typeof createWS> | null>(null)
  const connectedRef = useRef(false)
  const stopPresenceRef = useRef<(() => void) | null>(null)
  const speechRef = useRef<ReturnType<typeof createSpeechRecognizer> | null>(null)
  const speechActiveRef = useRef(false)
  const finalTranscriptRef = useRef('')
  const interimTranscriptRef = useRef('')
  const micStreamRef = useRef<MediaStream | null>(null)
  const silenceTimerRef = useRef<number | null>(null)

  const appendMessage = useCallback((msg: ChatMessage) => {
    setMessages((prev) => [...prev, msg])
  }, [])

  const handleChatMessage = useCallback(
    (raw: any) => {
      const nextId = () => {
        idRef.current += 1
        return idRef.current
      }
      if (!raw || typeof raw !== 'object') return
      switch (raw.type) {
        case 'message': {
          const text = typeof raw.text === 'string' ? raw.text : ''
          if (!text) return
          let role: 'user' | 'assistant' | 'system' = 'assistant'
          if (raw.role === 'user') role = 'user'
          else if (raw.role === 'system') role = 'system'
          const displayText = raw.role ? text : `(roleなし) ${text}`
          appendMessage({
            id: nextId(),
            type: role,
            text: displayText,
            responseId: raw.response_id,
            final: raw.final,
            source: typeof raw.source === 'string' ? raw.source : undefined,
          })
          break
        }
        case 'function_call': {
          appendMessage({
            id: nextId(),
            type: 'function_call',
            toolCallId: String(raw.tool_call_id || ''),
            name: String(raw.name || ''),
            args: raw.arguments ? JSON.stringify(raw.arguments) : undefined,
          })
          break
        }
        case 'function_result': {
          appendMessage({
            id: nextId(),
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
    [appendMessage],
  )

  const resetTranscripts = useCallback(() => {
    finalTranscriptRef.current = ''
    interimTranscriptRef.current = ''
    setTranscriptFinal('')
    setTranscriptInterim('')
  }, [])

  const sendVoiceText = useCallback(
    (text: string) => {
      const ws = wsChatRef.current
      const trimmed = text.trim()
      if (!ws || !connectedRef.current || !trimmed) return
      console.log('voice send', { text: trimmed, readyState: ws?.readyState })
      ws.send({ type: 'message', role: 'user', text: trimmed })
      appendMessage({ id: Date.now(), type: 'user', text: trimmed, responseId: undefined, final: true, source: 'browser' })
    },
    [appendMessage],
  )

  const flushTranscript = useCallback(() => {
    const finalText = finalTranscriptRef.current.trim()
    const interimText = interimTranscriptRef.current.trim()
    const text = finalText || interimText
    console.log('voice flush', { finalText, interimText })
    if (!text) return
    sendVoiceText(text)
    resetTranscripts()
  }, [resetTranscripts, sendVoiceText])

  const clearSilenceTimer = useCallback(() => {
    if (silenceTimerRef.current === null) return
    window.clearTimeout(silenceTimerRef.current)
    silenceTimerRef.current = null
  }, [])

  const scheduleSilenceFlush = useCallback(
    (delayMs: number) => {
      clearSilenceTimer()
      silenceTimerRef.current = window.setTimeout(() => {
        if (!speechActiveRef.current) return
        flushTranscript()
        setVoiceStatus('待機中')
      }, delayMs)
    },
    [clearSilenceTimer, flushTranscript],
  )

  const resetSilenceTimer = useCallback(() => {
    scheduleSilenceFlush(silenceTimeoutMs)
  }, [scheduleSilenceFlush])

  const stopVoice = useCallback(() => {
    speechActiveRef.current = false
    clearSilenceTimer()
    speechRef.current?.stop()
    speechRef.current = null
    if (micStreamRef.current) {
      micStreamRef.current.getTracks().forEach((t) => t.stop())
    }
    micStreamRef.current = null
    setVoiceStatus('停止中')
    setSpeechError('')
    resetTranscripts()
  }, [clearSilenceTimer, resetTranscripts])

  const startVoice = useCallback(async () => {
    if (speechRef.current) return
    console.log('voice start: begin')
    setSpeechError('')
    const speech = createSpeechRecognizer({
      onFinal: (text) => {
        console.log('speech final', text)
        const next = [finalTranscriptRef.current, text].filter(Boolean).join(' ').trim()
        finalTranscriptRef.current = next
        setTranscriptFinal(next)
        setTranscriptInterim('')
        setVoiceStatus('認識中')
        resetSilenceTimer()
      },
      onInterim: (text) => {
        console.log('speech interim', text)
        interimTranscriptRef.current = text
        setTranscriptInterim(text)
        setVoiceStatus('認識中')
        resetSilenceTimer()
      },
      onResult: () => {
        resetSilenceTimer()
      },
      onSpeechEnd: () => {
        scheduleSilenceFlush(200)
      },
      onSoundEnd: () => {
        scheduleSilenceFlush(200)
      },
      onStart: () => {
        console.log('speech start')
        setVoiceStatus('待機中')
      },
      onEnd: () => {
        console.log('speech end')
        if (!speechActiveRef.current) {
          setVoiceStatus('停止中')
          return
        }
        window.setTimeout(() => {
          if (speechActiveRef.current) {
            try {
              speech.start()
            } catch (err) {
              setSpeechError(err instanceof Error ? err.message : 'speech restart error')
            }
          }
        }, 200)
      },
      onError: (message) => {
        console.log('speech error', message)
        setSpeechError(message)
      },
    })

    if (!speech.isSupported) {
      console.log('speech unsupported')
      setSpeechError('SpeechRecognition 非対応ブラウザです')
      return
    }

    speechRef.current = speech
    speechActiveRef.current = true

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
      console.log('mic error', err)
      setSpeechError(err instanceof Error ? err.message : 'microphone error')
      speechActiveRef.current = false
      speechRef.current = null
      setVoiceStatus('停止中')
      return
    }
    micStreamRef.current = stream
    setVoiceStatus('待機中')
    try {
      speech.start()
    } catch (err) {
      setSpeechError(err instanceof Error ? err.message : 'speech start error')
    }
  }, [flushTranscript, resetSilenceTimer, resetTranscripts, scheduleSilenceFlush])

  const connect = useCallback(async () => {
    if (connected || busy) return
    setBusy(true)
    try {
      const wsAudio = createWS(audioWSUrl)
      const wsChat = createWS(chatWSUrl)

      await Promise.all([
        wsAudio.connect((msg) => {
          if (msg?.type === 'audio.play' && msg.audio) {
            receiver.play(msg.audio)
          }
        }),
        wsChat.connect((msg) => {
          handleChatMessage(msg)
        }),
      ])

      wsAudioRef.current = wsAudio
      wsChatRef.current = wsChat

      setConnected(true)
      connectedRef.current = true
      await startVoice()
      appendMessage({ id: Date.now(), type: 'assistant', text: '接続しました。話しかけてください。' })
    } catch (e) {
      console.error('connect error', e)
      stopVoice()
      setConnected(false)
      connectedRef.current = false
      wsAudioRef.current?.close()
      wsAudioRef.current = null
      wsChatRef.current?.close()
      wsChatRef.current = null
      throw e
    } finally {
      setBusy(false)
    }
  }, [appendMessage, busy, connected, handleChatMessage, receiver, startVoice, stopVoice])

  const disconnect = useCallback(() => {
    stopVoice()
    wsAudioRef.current?.close()
    wsAudioRef.current = null
    wsChatRef.current?.close()
    wsChatRef.current = null
    setConnected(false)
    connectedRef.current = false
  }, [stopVoice])

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
    ws.send({ type: "reset" })
  }, [connected])

  const sendText = useCallback(() => {
    const ws = wsChatRef.current
    const text = input.trim()
    if (!ws || !connected || !text) return
    const msg = { type: 'message', role: 'user', text }
    ws.send(msg)
    // ローカルにも即時反映
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
          リセット
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
          <strong>音声認識:</strong> {voiceStatus}
        </div>
        {speechError && (
          <div style={{ color: '#dc2626' }}>
            <strong>音声認識エラー:</strong> {speechError}
          </div>
        )}
        <div>
          <strong>文字起こし(確定):</strong> {transcriptFinal || '（なし）'}
        </div>
        <div style={{ color: '#6b7280' }}>
          <strong>文字起こし(途中):</strong> {transcriptInterim || '（なし）'}
        </div>
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
          const sourceLabel = m.source ? ` (${m.source})` : ''
          return (
            <div key={m.id} style={{ marginBottom: 8 }}>
              <strong style={{ color }}>{label}{sourceLabel}</strong>
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
