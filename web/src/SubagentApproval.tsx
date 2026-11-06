import React, { useEffect, useState } from 'react'

type Permission = { id: string; input: string; session_id: string; created_at: string }

export function SubagentApproval({ backendURL }: { backendURL: URL }) {
  const [requests, setRequests] = useState<Permission[]>([])
  const [error, setError] = useState('')
  const [deciding, setDeciding] = useState('')
  useEffect(() => {
    const controller = new AbortController()
    let pending = false
    const refresh = async () => {
      if (pending) return
      pending = true
      try {
        const response = await fetch(new URL('/api/subagent/permissions', backendURL), { signal: controller.signal, cache: 'no-store' })
        if (!response.ok) throw new Error('サブエージェントの許可要求を取得できませんでした')
        const data = await response.json() as { requests: Permission[] }
        if (!controller.signal.aborted) { setRequests(data.requests); setError('') }
      } catch (err) {
        if (!controller.signal.aborted) setError(err instanceof Error ? err.message : '許可要求を取得できませんでした')
      } finally { pending = false }
    }
    void refresh()
    const timer = window.setInterval(() => void refresh(), 1000)
    return () => { controller.abort(); window.clearInterval(timer) }
  }, [backendURL])
  const decide = async (request: Permission, approved: boolean) => {
    setDeciding(request.id)
    try {
      const response = await fetch(new URL(`/api/subagent/permissions/${encodeURIComponent(request.id)}`, backendURL), {
        method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ approved }),
      })
      if (!response.ok) throw new Error(await response.text() || '許可要求に回答できませんでした')
      setRequests(previous => previous.filter(item => item.id !== request.id))
      setError('')
    } catch (err) { setError(err instanceof Error ? err.message : '許可要求に回答できませんでした') }
    finally { setDeciding('') }
  }
  if (!requests.length) return error ? <div role="status" style={{ position: 'fixed', bottom: 4, left: 8, color: 'var(--ui-danger-text)', background: 'var(--live-panel)', padding: 4, zIndex: 20 }}>{error}</div> : null
  return (
    <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,.5)', display: 'grid', placeItems: 'center', zIndex: 100 }}>
      <section role="dialog" aria-modal="true" aria-labelledby="subagent-permission-title" style={{ width: 'min(92vw, 640px)', maxHeight: '85dvh', overflow: 'auto', padding: 20, borderRadius: 12, background: 'var(--live-panel)', color: 'var(--live-text)', border: '1px solid var(--live-line)' }}>
        <h2 id="subagent-permission-title">サブエージェントへの送信</h2>
        <p>次の依頼文を外部モデルに送信します。許可するまで送信しません。</p>
        <p>{requests[0].session_id ? `会話を継続: ${requests[0].session_id}` : '新しい会話を開始します'}</p>
        <pre style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', background: 'var(--live-panel-soft)', padding: 12 }}>{requests[0].input}</pre>
        <p>この依頼だけを許可します。2分で期限切れになります。</p>
        {error && <p role="alert">{error}</p>}
        <div style={{ display: 'flex', gap: 12 }}>
          <button className="live-admin-btn" disabled={!!deciding} onClick={() => void decide(requests[0], false)}>拒否する</button>
          <button className="live-admin-btn" disabled={!!deciding} onClick={() => void decide(requests[0], true)}>この依頼を許可する</button>
        </div>
      </section>
    </div>
  )
}
