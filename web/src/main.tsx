import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import ReactDOM from 'react-dom/client'
import { createAudioReceiver, createAudioSender } from './audio'
import { downloadLanguageModel, startCameraWatcher } from './vision'
import { createWS } from './ws'

type ChatMessage =
  | { id: number; type: 'user' | 'assistant' | 'system'; text: string; responseId?: string; final?: boolean; source?: string }
  | { id: number; type: 'function_call'; toolCallId: string; name: string; args?: string }
  | { id: number; type: 'function_result'; toolCallId: string; output?: string }

const audioWSUrl = 'ws://localhost:8081/ws/audio'
const chatWSUrl = 'ws://localhost:8081/ws/chat'

function App() {
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [connected, setConnected] = useState(false)
  const [busy, setBusy] = useState(false)
  const [input, setInput] = useState('')
  const [cameraEnabled, setCameraEnabled] = useState(false)
  const [cameraStatus, setCameraStatus] = useState('停止中')
  const [cameraSummary, setCameraSummary] = useState('')
  const [cameraChanged, setCameraChanged] = useState<'yes' | 'no' | ''>('')
  const [cameraError, setCameraError] = useState('')
  const [cameraUpdatedAt, setCameraUpdatedAt] = useState('')
  const [cameraPerson, setCameraPerson] = useState<'yes' | 'no' | ''>('')
  const [cameraActivity, setCameraActivity] = useState('')
  const [downloading, setDownloading] = useState(false)
  const idRef = useRef(0)
  const chatRef = useRef<HTMLDivElement | null>(null)
  const videoRef = useRef<HTMLVideoElement | null>(null)

  const receiver = useMemo(() => createAudioReceiver(), [])

  const wsAudioRef = useRef<ReturnType<typeof createWS> | null>(null)
  const wsChatRef = useRef<ReturnType<typeof createWS> | null>(null)
  const stopSenderRef = useRef<(() => void) | null>(null)
  const stopCameraRef = useRef<(() => void) | null>(null)

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
        wsChat.connect((msg) => handleChatMessage(msg)),
      ])

      wsAudioRef.current = wsAudio
      wsChatRef.current = wsChat

      const sender = await createAudioSender((audioB64) => {
        wsAudio.send({ type: 'audio.append', audio: audioB64 })
      })
      await sender.start()
      stopSenderRef.current = () => sender.stop()
      setConnected(true)
      appendMessage({ id: Date.now(), type: 'assistant', text: '接続しました。話しかけてください。' })
    } catch (e) {
      console.error('connect error', e)
      throw e
    } finally {
      setBusy(false)
    }
  }, [appendMessage, busy, connected, handleChatMessage, receiver])

  const disconnect = useCallback(() => {
    stopSenderRef.current?.()
    stopSenderRef.current = null
    wsAudioRef.current?.close()
    wsAudioRef.current = null
    wsChatRef.current?.close()
    wsChatRef.current = null
    setConnected(false)
  }, [])

  useEffect(() => {
    return () => {
      disconnect()
      stopCameraRef.current?.()
      stopCameraRef.current = null
    }
  }, [disconnect])

  const sendCameraContext = useCallback(
    (summary: string, changed: 'yes' | 'no', person: 'yes' | 'no', activity: string) => {
      const ws = wsChatRef.current
      if (!ws || !connected) return
      ws.send({
        type: 'camera_context',
        summary,
        changed,
        person,
        activity,
        timestamp: new Date().toISOString(),
      })
    },
    [connected],
  )

  const startCamera = useCallback(async () => {
    if (cameraEnabled) {
      stopCameraRef.current?.()
      stopCameraRef.current = null
      setCameraEnabled(false)
      setCameraStatus('停止中')
      setCameraError('')
      return
    }
    if (!videoRef.current) {
      return
    }
    setCameraEnabled(true)
    setCameraError('')
    try {
      const handle = await startCameraWatcher({
        video: videoRef.current,
        intervalMs: 10000, // 10秒ごとに解析
        onStatus: (status, message) => {
          if (status === 'error') {
            setCameraStatus('エラー')
            setCameraError(message ?? '')
            return
          }
          if (status === 'unsupported') {
            setCameraStatus('未対応')
            setCameraError(message ?? '')
            return
          }
          if (status === 'starting') {
            setCameraStatus(message ? `起動中: ${message}` : '起動中')
            return
          }
          if (status === 'ready') {
            setCameraStatus(message ? `待機中: ${message}` : '待機中')
            return
          }
          if (status === 'running') {
            setCameraStatus(message ? `解析中: ${message}` : '解析中')
            return
          }
          if (status === 'idle') {
            setCameraStatus('停止中')
          }
        },
        onResult: (result) => {
          setCameraSummary(result.summary)
          setCameraChanged(result.changed)
          setCameraUpdatedAt(result.timestamp)
          setCameraPerson(result.personPresent)
          setCameraActivity(result.activity)
          if (result.changed === 'yes' && result.summary) {
            sendCameraContext(result.summary, result.changed, result.personPresent, result.activity)
          }
        },
      })
      stopCameraRef.current = handle.stop
    } catch (err) {
      setCameraEnabled(false)
      setCameraStatus('エラー')
      setCameraError(err instanceof Error ? err.message : 'unknown error')
    }
  }, [cameraEnabled, sendCameraContext])

  const downloadModel = useCallback(async () => {
    if (downloading) return
    setDownloading(true)
    setCameraError('')
    try {
      await downloadLanguageModel((status, message) => {
        if (status === 'error') {
          setCameraStatus('エラー')
          setCameraError(message ?? '')
          return
        }
        if (status === 'unsupported') {
          setCameraStatus('未対応')
          setCameraError(message ?? '')
          return
        }
        if (status === 'starting') {
          setCameraStatus(message ? `起動中: ${message}` : '起動中')
          return
        }
        if (status === 'ready') {
          setCameraStatus(message ? `待機中: ${message}` : '待機中')
          return
        }
        if (status === 'running') {
          setCameraStatus(message ? `解析中: ${message}` : '解析中')
          return
        }
      })
      setCameraStatus('待機中')
    } catch (err) {
      setCameraStatus('エラー')
      setCameraError(err instanceof Error ? err.message : 'unknown error')
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
        <button onClick={startCamera} disabled={busy}>
          {cameraEnabled ? 'カメラ停止' : 'カメラ開始'}
        </button>
        <button onClick={downloadModel} disabled={downloading}>
          {downloading ? 'モデルDL中' : 'モデルDL開始'}
        </button>
      </div>
      <div style={{ display: 'flex', gap: 16, alignItems: 'flex-start' }}>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          <div>
            <strong>カメラ状態:</strong> {cameraStatus}
          </div>
          {cameraError && (
            <div style={{ color: '#dc2626' }}>
              <strong>エラー:</strong> {cameraError}
            </div>
          )}
          <video ref={videoRef} width={320} height={240} autoPlay muted style={{ transform: 'scaleX(-1)' }} />
        </div>
        <div style={{ flex: 1 }}>
          <div>
            <strong>直近の変化:</strong>
          </div>
          <div style={{ minHeight: 48, background: '#f3f4f6', borderRadius: 6, padding: 8 }}>
            {cameraSummary || '（なし）'}
          </div>
          <div style={{ marginTop: 8 }}>
            <strong>changed判定:</strong> {cameraChanged || '（なし）'}
          </div>
          <div style={{ marginTop: 8 }}>
            <strong>人の有無:</strong> {cameraPerson || '（なし）'}
          </div>
          <div style={{ marginTop: 8 }}>
            <strong>作業内容:</strong> {cameraActivity || '（なし）'}
          </div>
          <div style={{ marginTop: 8 }}>
            <strong>最終出力時刻:</strong> {cameraUpdatedAt || '（なし）'}
          </div>
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
