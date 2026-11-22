import React, { useCallback, useEffect, useRef, useState } from 'react'
import { apiURL, wsURL } from './shared/backend'
import { createSocket, type Socket } from './shared/socket'

// awakeは呼びかけ後の会話中で、最後の発話から設定された時間（既定5分）は呼びかけなしで話せる状態。
export type WakeMode = { required: boolean, awake: boolean }

export type WakeModeState = {
  // サーバーとつながっていない間は undefined。
  mode?: WakeMode
  error: string
  pending: boolean
  save: (required: boolean) => void
  // 呼びかけをonにしたうえで、呼びかけなしで話せる会話中の状態にする。
  wake: () => Promise<void>
}

// 呼びかけ設定をサーバーから購読する。メイン画面（キャラクターエリアの暗さ）と設定画面（トグル）で共有する。
export function useWakeMode(): WakeModeState {
  const [mode, setMode] = useState<WakeMode>()
  const [error, setError] = useState('')
  const [pending, setPending] = useState(false)
  const socketRef = useRef<Socket | null>(null)
  const openRef = useRef(false)
  const writing = useRef(false)
  const modeRef = useRef<WakeMode>()
  modeRef.current = mode

  useEffect(() => {
    const socket = createSocket(wsURL('/ws/wake-mode'), {
      onOpen: () => { openRef.current = true },
      onMessage: (next: { type?: string; required?: boolean; awake?: boolean; error?: string }) => {
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
      },
      onClose: () => {
        openRef.current = false
        writing.current = false
        setPending(false)
        setMode(undefined)
        setError('呼びかけ設定に再接続しています')
      },
      retryDelay: () => 2000,
    })
    socketRef.current = socket
    return () => {
      socketRef.current = null
      openRef.current = false
      socket.close()
    }
  }, [])

  const save = useCallback((required: boolean) => {
    const socket = socketRef.current
    if (writing.current || !socket || !openRef.current) return
    writing.current = true
    setPending(true)
    setError('')
    socket.send({ type: 'set_wake_mode', required })
  }, [])

  // 呼びかけをonにすると会話中の状態が解除されるため、onにし終えてから会話中にする。
  // 順番を保証するため、応答を待てるHTTPのAPIを使う。結果の状態はWebSocketで届く。
  const wake = useCallback(async () => {
    try {
      if (!modeRef.current?.required) {
        const response = await fetch(apiURL('/api/wake-mode'), {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ required: true }),
          cache: 'no-store',
        })
        if (!response.ok) throw new Error(await response.text())
      }
      const response = await fetch(apiURL('/api/wake-mode/wake'), { method: 'POST', cache: 'no-store' })
      if (!response.ok) throw new Error(await response.text())
    } catch {
      setError('呼びかけ中にできませんでした')
    }
  }, [])

  return { mode, error, pending, save, wake }
}

export function WakeModeControl({ state }: { state: WakeModeState }) {
  const { mode, error, pending, save } = state
  return (
    <div style={{ display: 'grid', gap: 4, minWidth: 0, fontSize: 12 }}>
      <button
        type="button"
        role="switch"
        aria-label="ウェイクアップワード必須"
        aria-checked={mode?.required ?? false}
        aria-busy={pending}
        className="live-control-btn"
        disabled={!mode || pending}
        onClick={() => { if (mode) save(!mode.required) }}
      >
        <span className={`live-toggle-switch ${mode?.required ? 'on' : ''}`} aria-hidden="true" />
        呼びかけ
      </button>
      {error && <span role="alert">{error}</span>}
    </div>
  )
}
