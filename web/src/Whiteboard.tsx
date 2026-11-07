import React, { useEffect, useMemo, useRef, useState } from 'react'

type BoardEntry = { id: number; content: string; timestamp: number }
type ToolEntry = {
  id: string
  call_id: string
  name: string
  arguments: string
  output: string
  status: 'running' | 'succeeded' | 'failed'
  started_at: string
  finished_at?: string
}
type BoardEvent =
  | { id: string; timestamp: number; kind: 'board'; entry: BoardEntry }
  | { id: string; timestamp: number; kind: 'call' | 'result'; entry: ToolEntry }

function prettyJSON(value: string) {
  if (!value) return '（なし）'
  try { return JSON.stringify(JSON.parse(value), null, 2) } catch { return value }
}

export function Whiteboard({ entries, backendURL }: { entries: BoardEntry[]; backendURL: URL }) {
  const [tools, setTools] = useState<ToolEntry[]>([])
  const [error, setError] = useState('')
  const boardRef = useRef<HTMLDivElement>(null)
  const follow = useRef(true)

  useEffect(() => {
    const controller = new AbortController()
    let pending = false
    const refresh = async () => {
      if (pending) return
      pending = true
      try {
        const response = await fetch(new URL('/api/tool-history', backendURL), { signal: controller.signal, cache: 'no-store' })
        if (!response.ok) throw new Error(`ツール履歴を取得できませんでした (${response.status})`)
        const data = await response.json() as { entries: ToolEntry[] }
        if (controller.signal.aborted) return
        setTools(previous => JSON.stringify(previous) === JSON.stringify(data.entries) ? previous : data.entries)
        setError('')
      } catch (err) {
        if (!controller.signal.aborted) setError(err instanceof Error ? err.message : 'ツール履歴を取得できませんでした')
      } finally {
        pending = false
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
    }
  }, [backendURL])

  const events = useMemo(() => {
    const result: BoardEvent[] = entries.map(entry => ({ id: `board-${entry.id}`, timestamp: entry.timestamp, kind: 'board', entry }))
    for (const entry of tools) {
      result.push({ id: `call-${entry.id}`, timestamp: Date.parse(entry.started_at), kind: 'call', entry })
      if (entry.status !== 'running') {
        result.push({ id: `result-${entry.id}`, timestamp: Date.parse(entry.finished_at || entry.started_at), kind: 'result', entry })
      }
    }
    return result.sort((a, b) => a.timestamp - b.timestamp)
  }, [entries, tools])

  useEffect(() => {
    const board = boardRef.current
    if (board && follow.current) board.scrollTop = board.scrollHeight
  }, [events])

  return (
    <div className="live-board" ref={boardRef} aria-label="ホワイトボード" onScroll={() => {
      const board = boardRef.current
      if (board) follow.current = board.scrollHeight - board.clientHeight - board.scrollTop < 32
    }}>
      <style>{`
        .live-board-event + .live-board-event { margin-top: 8px; }
        .live-board-event.live-board-entry + .live-board-entry { border-top: 1px solid var(--live-line); padding-top: 10px; }
        .live-tool-entry { min-width: 0; }
        .live-tool-entry > summary.live-tool-toast { animation: none; pointer-events: auto; cursor: pointer; list-style: none; }
        .live-tool-entry > summary::-webkit-details-marker { display: none; }
        .live-tool-entry > summary::marker { content: ''; }
        .live-tool-entry > summary:focus-visible { outline: 2px solid var(--live-toggle-on); outline-offset: 2px; }
        .live-tool-entry > summary.failed { background: var(--ui-danger-bg); border-color: var(--ui-danger-line); color: var(--ui-danger-text); }
        .live-tool-entry .live-tool-toast-tool { overflow-wrap: anywhere; white-space: normal; }
        .live-tool-entry-status { flex-shrink: 0; font-size: 10px; }
        .live-tool-entry-body { padding: 8px; border: 1px solid var(--live-line); border-radius: 8px; margin-top: 4px; overflow-wrap: anywhere; }
        .live-tool-entry-body p { margin: 0 0 8px; color: var(--live-muted); }
        .live-tool-entry-body h3 { margin: 8px 0 4px; font-size: 12px; }
        .live-tool-entry-body pre { margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; font-size: 11px; }
      `}</style>
      {error && <div role="alert">{error}</div>}
      {events.map(event => {
        if (event.kind === 'board') {
          return <div key={event.id} className="live-board-event live-board-entry">{event.entry.content}</div>
        }
        const entry = event.entry
        const status = event.kind === 'call' ? '呼び出し' : entry.status === 'failed' ? '失敗' : '完了'
        return (
          <details key={event.id} className="live-board-event live-tool-entry" onToggle={event => {
            if (event.currentTarget.open) follow.current = false
          }}>
            <summary className={`live-tool-toast ${event.kind}${event.kind === 'result' && entry.status === 'failed' ? ' failed' : ''}`}>
              <span className="live-tool-toast-tool">{event.kind}: {entry.name}</span>
              <span className="live-tool-entry-status">{status} ▾</span>
            </summary>
            <div className="live-tool-entry-body">
              <p>{new Date(event.timestamp).toLocaleString('ja-JP')}<br />呼び出しID: {entry.call_id || '（なし）'}</p>
              <h3>引数</h3>
              <pre>{prettyJSON(entry.arguments)}</pre>
              <h3>レスポンス</h3>
              <pre>{entry.status === 'running' ? '結果を待っています…' : prettyJSON(entry.output)}</pre>
            </div>
          </details>
        )
      })}
    </div>
  )
}
