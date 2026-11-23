import React, { useEffect, useState } from 'react'
import { PageLayout } from '../../shared/ui/PageLayout'
import { createJob, deleteJob, listJobs, updateJob, type AutomationSource, type Job, type JobSpec } from './api'
import { describeSchedule, formatDateTime, scheduleKind } from './schedule'
import './AutomationScreen.css'

// 予約ジョブを一覧・追加・編集・削除する画面。開いたときと「更新」を押したときに一覧を取得する。
export function AutomationScreen({ onBack }: { onBack: () => void }) {
  const [jobs, setJobs] = useState<Job[]>([])
  const [sources, setSources] = useState<AutomationSource[]>([])
  const [error, setError] = useState('')
  const [revision, setRevision] = useState(0)
  const [pending, setPending] = useState(false)
  const [editor, setEditor] = useState<{ id?: string; spec: JobSpec }>()
  useEffect(() => {
    const controller = new AbortController()
    listJobs(controller.signal)
      .then((data) => {
        if (controller.signal.aborted) return
        setJobs(data.jobs)
        setSources(data.sources)
        setError('')
      })
      .catch((err) => {
        if (!controller.signal.aborted) setError(err instanceof Error ? err.message : '予約ジョブを取得できませんでした')
      })
    return () => controller.abort()
  }, [revision])
  const mutate = async (operation: () => Promise<void>) => {
    setPending(true)
    try {
      await operation()
      setError('')
      setEditor(undefined)
      setRevision(value => value + 1)
    } catch (err) {
      setError(err instanceof Error ? err.message : '予約ジョブを更新できませんでした')
    } finally {
      setPending(false)
    }
  }
  const save = (target: { id?: string; spec: JobSpec }) => {
    const { id, spec } = target
    void mutate(() => id ? updateJob(id, spec) : createJob(spec))
  }
  const remove = (job: Job) => {
    if (window.confirm(`「${job.name}」を削除しますか？`)) void mutate(() => deleteJob(job.id))
  }
  const patchSpec = (changes: Partial<JobSpec>) => setEditor(previous => previous ? { ...previous, spec: { ...previous.spec, ...changes } } : previous)
  return (
    <PageLayout onBack={onBack}>
      <section aria-label="予約ジョブ" className="jobs-view">
        <div className="jobs-header">
          <h1>予約</h1>
          <button className="job-btn plain" onClick={() => setRevision(value => value + 1)}>更新</button>
          <button className="job-btn primary" onClick={() => setEditor({ spec: { name: '', script: '', schedule: { cron: '0 8 * * 1-5', timezone: 'Asia/Tokyo' } } })}>追加</button>
        </div>
        <p className="jobs-hint">AIに「10分後に起こして」「平日8時に処理して。音声通知は不要」などと依頼して登録できます。日時指定の予約は実行後に削除されます。</p>
        {error && <p role="alert" className="job-error">{error}</p>}
        {editor && <form className="job-card job-editor" onSubmit={event => { event.preventDefault(); save(editor) }}>
          <label>名前<input required value={editor.spec.name} onChange={event => patchSpec({ name: event.currentTarget.value })} /></label>
          <label>スクリプト<textarea className="job-script-input" required rows={6} value={editor.spec.script} onChange={event => patchSpec({ script: event.currentTarget.value })} /></label>
          <label>予約の種類<select value={scheduleKind(editor.spec.schedule)} onChange={event => { const kind = event.currentTarget.value; patchSpec({ schedule: kind === 'cron' ? { cron: '0 8 * * 1-5', timezone: 'Asia/Tokyo' } : kind === 'source' ? { source: sources[0]?.name ?? '' } : { at: '' } }) }}><option value="cron">繰り返し（cron）</option><option value="at">日時指定（1回）</option><option value="source">記録が届いたとき</option></select></label>
          {scheduleKind(editor.spec.schedule) === 'source' ? <label>記録の種類<select required value={editor.spec.schedule.source ?? ''} onChange={event => patchSpec({ schedule: { source: event.currentTarget.value } })}>
            <option value="" disabled>選んでください</option>
            {sources.map(e => <option key={e.name} value={e.name}>{e.description}（{e.name}）</option>)}
            {editor.spec.schedule.source && !sources.some(e => e.name === editor.spec.schedule.source) && <option value={editor.spec.schedule.source}>使えない記録の種類（{editor.spec.schedule.source}）</option>}
          </select></label> : editor.spec.schedule.cron !== undefined ? <>
            <label>cron（分 時 日 月 曜日）<input required value={editor.spec.schedule.cron} onChange={event => patchSpec({ schedule: { ...editor.spec.schedule, cron: event.currentTarget.value } })} /></label>
            <label>タイムゾーン<input required value={editor.spec.schedule.timezone ?? 'Asia/Tokyo'} onChange={event => patchSpec({ schedule: { ...editor.spec.schedule, timezone: event.currentTarget.value } })} /></label>
          </> : <label>日時（例: 2026-11-10T08:00:00+09:00）<input required value={editor.spec.schedule.at ?? ''} onChange={event => patchSpec({ schedule: { at: event.currentTarget.value } })} /></label>}
          <div className="job-actions end">
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
    </PageLayout>
  )
}
