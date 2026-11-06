import React, { useEffect, useState } from 'react'

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

function prettyJSON(value: string) {
  if (!value) return '（なし）'
  try { return JSON.stringify(JSON.parse(value), null, 2) } catch { return value }
}

export function ToolHistoryView({ visible, backendURL, goApp }: { visible: boolean; backendURL: URL; goApp: () => void }) {
  const [entries, setEntries] = useState<ToolEntry[]>([])
  const [limit, setLimit] = useState(200)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [revision, setRevision] = useState(0)
  useEffect(() => {
    if (!visible) return
    const controller = new AbortController()
    let pending = false
    const refresh = async () => {
      if (pending) return
      pending = true
      try {
        const response = await fetch(new URL('/api/tool-history', backendURL), { signal: controller.signal, cache: 'no-store' })
        if (!response.ok) throw new Error(`ツール履歴を取得できませんでした (${response.status})`)
        const data = await response.json() as { entries: ToolEntry[]; limit: number }
        if (controller.signal.aborted) return
        setEntries(data.entries)
        setLimit(data.limit)
        setError('')
      } catch (err) {
        if (!controller.signal.aborted) setError(err instanceof Error ? err.message : 'ツール履歴を取得できませんでした')
      } finally {
        pending = false
        if (!controller.signal.aborted) setLoading(false)
      }
    }
    setLoading(true)
    void refresh()
    const timer = window.setInterval(() => void refresh(), 2000)
    return () => { controller.abort(); window.clearInterval(timer) }
  }, [visible, backendURL, revision])

  const statusLabels = { running: '実行中', succeeded: '成功', failed: '失敗' }
  return (
    <section style={{ display: visible ? 'flex' : 'none', flexDirection: 'column', height: '100dvh', gap: 12, padding: 12, overflow: 'auto' }} aria-label="ツール呼び出し履歴">
      <style>{`
        .tool-history-entry { border: 1px solid var(--live-line); border-radius: 10px; padding: 12px; background: var(--live-panel); }
        .tool-history-entry summary { cursor: pointer; overflow-wrap: anywhere; }
        .tool-history-body { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
        .tool-history-body pre { white-space: pre-wrap; overflow-wrap: anywhere; background: var(--live-panel-soft); border: 1px solid var(--live-line); padding: 10px; border-radius: 8px; }
        @media (max-width: 640px) { .tool-history-body { grid-template-columns: 1fr; } }
      `}</style>
      <div style={{ display: 'flex', gap: 12, alignItems: 'center', flexWrap: 'wrap' }}>
        <h1 style={{ margin: 0, fontSize: 20 }}>ツール呼び出し履歴</h1>
        <button className="live-admin-btn" onClick={goApp}>app画面</button>
        <button className="live-admin-btn" onClick={() => setRevision(value => value + 1)}>更新</button>
      </div>
      <div style={{ color: 'var(--live-muted)', fontSize: 13 }}>最新{limit}件・新しい順。サーバー再起動で消去されます。</div>
      {error && <div role="alert" style={{ color: 'var(--ui-danger-text)' }}>{error}</div>}
      {loading && <div role="status">読み込み中…</div>}
      {!loading && !error && entries.length === 0 && <div>ツール呼び出し履歴はありません</div>}
      {entries.map(entry => (
        <details className="tool-history-entry" key={entry.id}>
          <summary>{entry.name} · {statusLabels[entry.status]} · {new Date(entry.started_at).toLocaleString('ja-JP')}</summary>
          <p style={{ color: 'var(--live-muted)', overflowWrap: 'anywhere' }}>呼び出しID: {entry.call_id || '（なし）'}</p>
          {entry.finished_at && <p>完了: {new Date(entry.finished_at).toLocaleString('ja-JP')}</p>}
          <div className="tool-history-body">
            <div><h2 style={{ fontSize: 15 }}>引数</h2><pre>{prettyJSON(entry.arguments)}</pre></div>
            <div><h2 style={{ fontSize: 15 }}>結果・返り値</h2><pre>{entry.status === 'running' ? '結果を待っています…' : prettyJSON(entry.output)}</pre></div>
          </div>
        </details>
      ))}
    </section>
  )
}
