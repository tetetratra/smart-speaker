import React, { useEffect, useState } from 'react'

type JobSpec = { name: string; instruction: string; schedule: { cron?: string; at?: string; after_seconds?: number; timezone?: string }; speak: boolean }
type Job = JobSpec & { id: string; enabled: boolean; next_run_at?: string; last_run_at?: string; last_status: string; last_error?: string }

export function AutomationView({ visible, backendURL, goApp }: { visible: boolean; backendURL: URL; goApp: () => void }) {
  const [jobs, setJobs] = useState<Job[]>([])
  const [error, setError] = useState('')
  const [revision, setRevision] = useState(0)
  const [pending, setPending] = useState(false)
  const [editor, setEditor] = useState<{ id?: string; spec: JobSpec }>()
  useEffect(() => {
    if (!visible) return
    const controller = new AbortController()
    let loading = false
    const refresh = async () => {
      if (loading) return
      loading = true
      try {
        const response = await fetch(new URL('/api/jobs', backendURL), { signal: controller.signal, cache: 'no-store' })
        if (!response.ok) throw new Error('予約ジョブを取得できませんでした')
        const data = await response.json() as { jobs: Job[] }
        if (!controller.signal.aborted) { setJobs(data.jobs); setError('') }
      } catch (err) { if (!controller.signal.aborted) setError(err instanceof Error ? err.message : '取得に失敗しました') }
      finally { loading = false }
    }
    void refresh()
    const timer = window.setInterval(() => void refresh(), 5000)
    return () => { controller.abort(); window.clearInterval(timer) }
  }, [visible, backendURL, revision])
  const mutate = async (method: string, id?: string, body?: unknown) => {
    setPending(true)
    try {
      const response = await fetch(new URL(id ? `/api/jobs/${encodeURIComponent(id)}` : '/api/jobs', backendURL), {
        method, headers: body ? { 'Content-Type': 'application/json' } : undefined, body: body ? JSON.stringify(body) : undefined,
      })
      if (!response.ok) throw new Error(await response.text() || '予約ジョブを更新できませんでした')
      setError(''); setEditor(undefined); setRevision(value => value + 1)
    } catch (err) { setError(err instanceof Error ? err.message : '更新に失敗しました') }
    finally { setPending(false) }
  }
  const patchSpec = (changes: Partial<JobSpec>) => setEditor(previous => previous ? { ...previous, spec: { ...previous.spec, ...changes } } : previous)
  const status: Record<string, string> = { scheduled: '予約済み', queued: '受付待ち', submitted: 'AIへ指示を送信済み', failed: '指示の送信失敗' }
  return (
    <section aria-label="予約ジョブ" style={{ display: visible ? 'flex' : 'none', flexDirection: 'column', gap: 12, height: '100dvh', overflow: 'auto', padding: 12 }}>
      <style>{`.job-card { background:var(--live-panel); border:1px solid var(--live-line); border-radius:10px; padding:12px; } .job-editor { display:grid; gap:10px; } .job-editor label { display:grid; gap:4px; } .job-editor input, .job-editor textarea { width:100%; padding:8px; border:1px solid var(--live-line-strong); border-radius:6px; }`}</style>
      <div style={{ display: 'flex', gap: 12, alignItems: 'center', flexWrap: 'wrap' }}>
        <h1 style={{ margin: 0, fontSize: 20 }}>予約ジョブ</h1>
        <button className="live-admin-btn" onClick={goApp}>app画面</button>
        <button className="live-admin-btn" onClick={() => setRevision(value => value + 1)}>更新</button>
        <button className="live-admin-btn" onClick={() => setEditor({ spec: { name: '', instruction: '', schedule: { cron: '0 8 * * 1-5', timezone: 'Asia/Tokyo' }, speak: false } })}>手動で追加</button>
      </div>
      <p style={{ margin: 0, color: 'var(--live-muted)' }}>AIに「10分後に起こして」「平日8時に処理して。音声通知は不要」などと依頼して登録できます。予約は再起動後も保持されます。</p>
      {error && <p role="alert">{error}</p>}
      {editor && <form className="job-card job-editor" onSubmit={event => { event.preventDefault(); void mutate(editor.id ? 'PUT' : 'POST', editor.id, editor.spec) }}>
        <label>名前<input required value={editor.spec.name} onChange={event => patchSpec({ name: event.currentTarget.value })} /></label>
        <label>実行する指示<textarea required rows={4} value={editor.spec.instruction} onChange={event => patchSpec({ instruction: event.currentTarget.value })} /></label>
        <label>予約の種類<select value={editor.spec.schedule.cron ? 'cron' : 'at'} onChange={event => patchSpec({ schedule: event.currentTarget.value === 'cron' ? { cron: '0 8 * * 1-5', timezone: 'Asia/Tokyo' } : { at: '' } })}><option value="cron">繰り返し（cron）</option><option value="at">日時指定（1回）</option></select></label>
        {editor.spec.schedule.cron !== undefined ? <>
          <label>cron（分 時 日 月 曜日）<input required value={editor.spec.schedule.cron} onChange={event => patchSpec({ schedule: { ...editor.spec.schedule, cron: event.currentTarget.value } })} /></label>
          <label>タイムゾーン<input required value={editor.spec.schedule.timezone ?? 'Asia/Tokyo'} onChange={event => patchSpec({ schedule: { ...editor.spec.schedule, timezone: event.currentTarget.value } })} /></label>
        </> : <label>日時（例: 2026-11-10T08:00:00+09:00）<input required value={editor.spec.schedule.at ?? ''} onChange={event => patchSpec({ schedule: { at: event.currentTarget.value } })} /></label>}
        <label style={{ display: 'flex', gap: 8 }}><input style={{ width: 'auto' }} type="checkbox" checked={editor.spec.speak} onChange={event => patchSpec({ speak: event.currentTarget.checked })} />音声で知らせる</label>
        <div style={{ display: 'flex', gap: 8 }}><button disabled={pending}>保存</button><button type="button" disabled={pending} onClick={() => setEditor(undefined)}>キャンセル</button></div>
      </form>}
      {jobs.length === 0 && !error && <p>登録済みの予約ジョブはありません</p>}
      {jobs.map(job => <article className="job-card" key={job.id}>
        <h2 style={{ fontSize: 17, marginTop: 0 }}>{job.name} · {job.enabled ? '有効' : '停止中'}</h2>
        <p style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>{job.instruction}</p>
        <p>音声通知: {job.speak ? 'あり' : 'なし'} ／ 次回: {job.next_run_at ? new Date(job.next_run_at).toLocaleString('ja-JP') : 'なし'}</p>
        <p style={{ overflowWrap: 'anywhere' }}>予定: {job.schedule.cron ?? job.schedule.at} {job.schedule.timezone ?? ''}</p>
        <p>{status[job.last_status] ?? job.last_status}{job.last_run_at ? ` · ${new Date(job.last_run_at).toLocaleString('ja-JP')}` : ''}</p>
        {job.last_error && <p role="alert">{job.last_error}</p>}
        <div style={{ display: 'flex', gap: 8 }}>
          <button className="live-admin-btn" disabled={pending} onClick={() => void mutate('PATCH', job.id, { enabled: !job.enabled })}>{job.enabled ? '停止' : '再開'}</button>
          <button className="live-admin-btn" disabled={pending} onClick={() => setEditor({ id: job.id, spec: { name: job.name, instruction: job.instruction, schedule: { ...job.schedule }, speak: job.speak } })}>編集</button>
          <button className="live-admin-btn" disabled={pending} onClick={() => void mutate('DELETE', job.id)}>削除</button>
        </div>
      </article>)}
    </section>
  )
}
