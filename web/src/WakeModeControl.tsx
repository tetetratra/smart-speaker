import React, { useEffect, useRef, useState } from 'react'

type WakeMode = { required: boolean }

export function WakeModeControl({ backendURL }: { backendURL: URL }) {
  const [mode, setMode] = useState<WakeMode>()
  const [error, setError] = useState('')
  const [pending, setPending] = useState(false)
  const writing = useRef(false)
  const sequence = useRef(0)

  useEffect(() => {
    const controller = new AbortController()
    let loading = false
    const refresh = async () => {
      if (loading || writing.current) return
      loading = true
      const id = ++sequence.current
      try {
        const response = await fetch(new URL('/api/wake-mode', backendURL), { signal: controller.signal, cache: 'no-store' })
        if (!response.ok) throw new Error('ウェイクアップワードの設定を取得できませんでした')
        const next = await response.json() as WakeMode
        if (id !== sequence.current || controller.signal.aborted) return
        setMode(next)
        setError('')
      } catch (err) {
        if (!controller.signal.aborted && id === sequence.current) {
          setError(err instanceof Error ? err.message : '設定を取得できませんでした')
        }
      } finally {
        loading = false
      }
    }
    const onFocus = () => { void refresh() }
    void refresh()
    const timer = window.setInterval(onFocus, 2000)
    window.addEventListener('focus', onFocus)
    return () => {
      controller.abort()
      window.clearInterval(timer)
      window.removeEventListener('focus', onFocus)
      sequence.current++
    }
  }, [backendURL])

  const save = async (required: boolean) => {
    if (writing.current) return
    writing.current = true
    setPending(true)
    const id = ++sequence.current
    try {
      const response = await fetch(new URL('/api/wake-mode', backendURL), {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ required }),
      })
      if (!response.ok) throw new Error(await response.text() || 'ウェイクアップワードの設定を更新できませんでした')
      const next = await response.json() as WakeMode
      if (id === sequence.current) {
        setMode(next)
        setError('')
      }
    } catch (err) {
      if (id === sequence.current) setError(err instanceof Error ? err.message : '設定を更新できませんでした')
    } finally {
      writing.current = false
      if (id === sequence.current) setPending(false)
    }
  }

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
        onClick={() => { if (mode) void save(!mode.required) }}
      >
        <span className={`live-toggle-switch ${mode?.required ? 'on' : ''}`} aria-hidden="true" />
        呼びかけ
      </button>
      {error && <span role="alert">{error}</span>}
    </div>
  )
}
