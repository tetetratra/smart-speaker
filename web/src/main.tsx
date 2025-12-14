import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import ReactDOM from 'react-dom/client'
import { createAudioReceiver, createAudioSender } from './audio'
import { createWS } from './ws'

type ChatMessage =
  | { id: number; type: 'user' | 'assistant'; text: string; responseId?: string; final?: boolean }
  | { id: number; type: 'function_call'; toolCallId: string; name: string; args?: string }
  | { id: number; type: 'function_result'; toolCallId: string; output?: string }

const audioWSUrl = 'ws://localhost:8081/ws/audio'
const chatWSUrl = 'ws://localhost:8081/ws/chat'

function App() {
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [connected, setConnected] = useState(false)
  const [busy, setBusy] = useState(false)
  const idRef = useRef(0)
  const chatRef = useRef<HTMLDivElement | null>(null)

  const receiver = useMemo(() => createAudioReceiver(), [])

  const wsAudioRef = useRef<ReturnType<typeof createWS> | null>(null)
  const wsChatRef = useRef<ReturnType<typeof createWS> | null>(null)
  const stopSenderRef = useRef<(() => void) | null>(null)

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
          let role: 'user' | 'assistant' = 'assistant'
          if (raw.role === 'user') role = 'user'
          const displayText = raw.role ? text : `(roleなし) ${text}`
          appendMessage({ id: nextId(), type: role, text: displayText, responseId: raw.response_id, final: raw.final })
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
    }
  }, [disconnect])

  useEffect(() => {
    const el = chatRef.current
    if (el) {
      el.scrollTop = el.scrollHeight
    }
  }, [messages])

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
          const color = m.type === 'user' ? '#2563eb' : '#16a34a'
          const label = m.type === 'user' ? 'User' : 'Assistant'
          return (
            <div key={m.id} style={{ marginBottom: 8 }}>
              <strong style={{ color }}>{label}</strong>
              <div>{m.text}</div>
            </div>
          )
        })}
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
