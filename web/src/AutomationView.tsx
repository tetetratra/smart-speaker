import React, { useEffect, useState } from 'react'

type JobSpec = { name: string; script: string; schedule: { cron?: string; at?: string; after_seconds?: number; timezone?: string } }
type Job = JobSpec & { id: string; next_run_at?: string; last_run_at?: string; last_status: string; last_error?: string }

const weekdays = ['日', '月', '火', '水', '木', '金', '土']
const pad = (value: number) => String(value).padStart(2, '0')
const formatDateTime = (value: string) => {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return `${date.getMonth() + 1}/${date.getDate()}(${weekdays[date.getDay()]}) ${date.getHours()}:${pad(date.getMinutes())}`
}
// よく使う形のcronだけ読みやすくし、それ以外は式をそのまま表示する。
const describeCron = (cron: string) => {
  const fields = cron.trim().split(/\s+/)
  if (fields.length !== 5) return cron
  const [minute, hour, dom, month, dow] = fields
  if (!/^\d+$/.test(minute) || !/^\d+$/.test(hour) || dom !== '*' || month !== '*') return cron
  const time = `${Number(hour)}:${pad(Number(minute))}`
  if (dow === '*') return `毎日 ${time}`
  if (dow === '1-5') return `平日 ${time}`
  if (dow === '0,6' || dow === '6,0') return `土日 ${time}`
  if (/^[0-7](,[0-7])*$/.test(dow)) return `毎週${dow.split(',').map(day => weekdays[Number(day) % 7]).join('・')} ${time}`
  return cron
}
const describeSchedule = (job: Job) => job.schedule.cron ? describeCron(job.schedule.cron) : job.schedule.at ? formatDateTime(job.schedule.at) : ''

const automationStyle = `
  .jobs-view { display:flex; flex-direction:column; gap:10px; height:100dvh; overflow:auto; padding:12px; background:var(--live-bg); color:var(--live-text); box-sizing:border-box; }
  .jobs-header { display:flex; gap:8px; align-items:center; flex-wrap:wrap; }
  .jobs-header h1 { margin:0 auto 0 0; font-size:18px; }
  .jobs-hint { margin:0; font-size:12px; color:var(--live-muted); }
  .jobs-list { display:flex; flex-direction:column; gap:6px; margin:0; padding:0; list-style:none; }
  .job-card { background:var(--live-panel); border:1px solid var(--live-line); border-radius:10px; padding:8px 10px; }
  .job-row { display:flex; align-items:center; gap:8px; min-width:0; }
  .job-main { flex:1 1 auto; min-width:0; display:flex; flex-direction:column; gap:2px; }
  .job-title { display:flex; align-items:center; gap:6px; min-width:0; }
  .job-name { font-size:14px; font-weight:700; margin:0; white-space:nowrap; overflow:hidden; text-overflow:ellipsis; }
  .job-meta { font-size:12px; color:var(--live-muted); white-space:nowrap; overflow:hidden; text-overflow:ellipsis; }
  .job-script { font-family:monospace; white-space:pre-wrap; font-size:12px; color:var(--live-muted); margin:4px 0 0; overflow:hidden; display:-webkit-box; -webkit-line-clamp:2; -webkit-box-orient:vertical; overflow-wrap:anywhere; }
  .job-badge { flex:0 0 auto; font-size:10px; font-weight:700; padding:2px 6px; border-radius:999px; background:var(--live-panel-soft); border:1px solid var(--live-line); color:var(--live-muted); }
  .job-badge.danger { background:var(--ui-danger-bg); border-color:var(--ui-danger-line); color:var(--ui-danger-text); }
  .job-actions { flex:0 0 auto; display:flex; gap:6px; }
  .job-btn { min-height:30px; padding:0 12px; border-radius:999px; font-size:12px; font-weight:700; cursor:pointer; border:1px solid transparent; }
  .job-btn:disabled { opacity:.5; cursor:default; }
  .job-btn.primary { background:var(--ui-accent); color:var(--ui-accent-text); }
  .job-btn.edit { background:var(--ui-accent-soft); color:var(--ui-accent-soft-text); }
  .job-btn.danger { background:var(--ui-danger-bg); border-color:var(--ui-danger-line); color:var(--ui-danger-text); }
  .job-btn.plain { background:var(--live-panel-soft); border-color:var(--live-line); color:var(--live-muted); }
  .job-error { margin:4px 0 0; font-size:12px; color:var(--ui-danger-text); overflow-wrap:anywhere; }
  .job-editor { display:grid; gap:10px; }
  .job-editor label { display:grid; gap:4px; font-size:12px; color:var(--live-muted); }
  .job-editor input, .job-editor textarea, .job-editor select { width:100%; box-sizing:border-box; padding:8px; border:1px solid var(--live-line-strong); border-radius:6px; background:var(--live-panel-soft); color:var(--live-text); font:inherit; font-size:14px; }
`

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
  const remove = (job: Job) => { if (window.confirm(`「${job.name}」を削除しますか？`)) void mutate('DELETE', job.id) }
  const patchSpec = (changes: Partial<JobSpec>) => setEditor(previous => previous ? { ...previous, spec: { ...previous.spec, ...changes } } : previous)
  return (
    <section aria-label="予約ジョブ" className="jobs-view" style={{ display: visible ? 'flex' : 'none' }}>
      <style>{automationStyle}</style>
      <div className="jobs-header">
        <h1>予約</h1>
        <button className="job-btn plain" onClick={goApp}>app画面</button>
        <button className="job-btn plain" onClick={() => setRevision(value => value + 1)}>更新</button>
        <button className="job-btn primary" onClick={() => setEditor({ spec: { name: '', script: '', schedule: { cron: '0 8 * * 1-5', timezone: 'Asia/Tokyo' } } })}>追加</button>
      </div>
      <p className="jobs-hint">AIに「10分後に起こして」「平日8時に処理して。音声通知は不要」などと依頼して登録できます。日時指定の予約は実行後に削除されます。</p>
      {error && <p role="alert" className="job-error">{error}</p>}
      {editor && <form className="job-card job-editor" onSubmit={event => { event.preventDefault(); void mutate(editor.id ? 'PUT' : 'POST', editor.id, editor.spec) }}>
        <label>名前<input required value={editor.spec.name} onChange={event => patchSpec({ name: event.currentTarget.value })} /></label>
        <label>スクリプト<textarea style={{ fontFamily: 'monospace' }} required rows={6} value={editor.spec.script} onChange={event => patchSpec({ script: event.currentTarget.value })} /></label>
        <label>予約の種類<select value={editor.spec.schedule.cron !== undefined ? 'cron' : 'at'} onChange={event => patchSpec({ schedule: event.currentTarget.value === 'cron' ? { cron: '0 8 * * 1-5', timezone: 'Asia/Tokyo' } : { at: '' } })}><option value="cron">繰り返し（cron）</option><option value="at">日時指定（1回）</option></select></label>
        {editor.spec.schedule.cron !== undefined ? <>
          <label>cron（分 時 日 月 曜日）<input required value={editor.spec.schedule.cron} onChange={event => patchSpec({ schedule: { ...editor.spec.schedule, cron: event.currentTarget.value } })} /></label>
          <label>タイムゾーン<input required value={editor.spec.schedule.timezone ?? 'Asia/Tokyo'} onChange={event => patchSpec({ schedule: { ...editor.spec.schedule, timezone: event.currentTarget.value } })} /></label>
        </> : <label>日時（例: 2026-11-10T08:00:00+09:00）<input required value={editor.spec.schedule.at ?? ''} onChange={event => patchSpec({ schedule: { at: event.currentTarget.value } })} /></label>}
        <div className="job-actions" style={{ justifyContent: 'flex-end' }}>
          <button type="button" className="job-btn plain" disabled={pending} onClick={() => setEditor(undefined)}>キャンセル</button>
          <button className="job-btn primary" disabled={pending}>保存</button>
        </div>
      </form>}
      {jobs.length === 0 && !error && <p className="jobs-hint">登録済みの予約はありません</p>}
      <ul className="jobs-list">
        {jobs.map(job => <li className="job-card" key={job.id}>
          <div className="job-row">
            <div className="job-main">
              <div className="job-title">
                <h2 className="job-name">{job.name}</h2>
                {job.last_status === 'failed' && <span className="job-badge danger">実行失敗</span>}
              </div>
              <span className="job-meta">
                {describeSchedule(job)}
                {job.schedule.cron && job.next_run_at ? ` · 次回 ${formatDateTime(job.next_run_at)}` : ''}
              </span>
            </div>
            <div className="job-actions">
              <button className="job-btn edit" disabled={pending} onClick={() => setEditor({ id: job.id, spec: { name: job.name, script: job.script, schedule: { ...job.schedule } } })}>編集</button>
              <button className="job-btn danger" disabled={pending} onClick={() => remove(job)}>削除</button>
            </div>
          </div>
          <p className="job-script" title={job.script}>{job.script}</p>
          {job.last_error && <p role="alert" className="job-error">{job.last_error}</p>}
        </li>)}
      </ul>
    </section>
  )
}
