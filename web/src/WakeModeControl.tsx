import React, { useEffect, useRef, useState } from 'react'

// awakeは呼びかけ後の会話中で、最後の発話から設定された時間（既定5分）は呼びかけなしで話せる状態。
type WakeMode = { required: boolean, awake?: boolean }

export function WakeModeControl({ backendURL }: { backendURL: URL }) {
  const [mode, setMode] = useState<WakeMode>()
  const [error, setError] = useState('')
  const [pending, setPending] = useState(false)
  const socketRef = useRef<WebSocket | null>(null)
  const writing = useRef(false)

  useEffect(() => {
    let disposed = false
    let timer: number | undefined
    let socket: WebSocket
    const connect = () => {
      const url = new URL('/ws/wake-mode', backendURL)
      url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
      socket = new WebSocket(url)
      socketRef.current = socket
      socket.onmessage = event => {
        if (disposed) return
        let next: { type?: string; required?: boolean; awake?: boolean; error?: string }
        try { next = JSON.parse(event.data) } catch { return }
        if (next.type !== 'wake_mode' && next.type !== 'wake_mode_result') return
        if (typeof next.required !== 'boolean' || typeof next.awake !== 'boolean') return
        setMode({ required: next.required, awake: next.awake })
        if (next.type === 'wake_mode_result') {
          writing.current = false
          setPending(false)
          setError(next.error || '')
        } else if (!writing.current) {
          setError('')
        }
      }
      socket.onclose = () => {
        if (disposed) return
        socketRef.current = null
        writing.current = false
        setPending(false)
        setMode(undefined)
        setError('呼びかけ設定に再接続しています')
        timer = window.setTimeout(connect, 2000)
      }
      socket.onerror = () => socket.close()
    }
    connect()
    return () => {
      disposed = true
      window.clearTimeout(timer)
      socketRef.current = null
      socket.close()
    }
  }, [backendURL])

  const save = (required: boolean) => {
    const socket = socketRef.current
    if (writing.current || !socket || socket.readyState !== WebSocket.OPEN) return
    writing.current = true
    setPending(true)
    setError('')
    socket.send(JSON.stringify({ type: 'set_wake_mode', required }))
  }

  return (
    <div style={{ display: 'grid', gap: 4, minWidth: 0, fontSize: 12 }}>
      <button
        type="button"
        role="switch"
        aria-label="ウェイクアップワード必須"
        aria-checked={mode?.required ?? false}
        aria-busy={pending}
        className={`live-control-btn ${mode?.required && mode.awake ? 'live-control-btn-awake' : ''}`}
        disabled={!mode || pending}
        onClick={() => { if (mode) void save(!mode.required) }}
      >
        <span className={`live-toggle-switch ${mode?.required ? 'on' : ''}`} aria-hidden="true" />
        呼びかけ
      </button>
      {error && <span role="alert">{error}</span>}
    </div>
  )
}
