import { requestJSON } from '../../shared/backend'

export type Schedule = { cron?: string; at?: string; after_seconds?: number; source?: string; timezone?: string }
export type JobSpec = { name: string; script: string; schedule: Schedule }
// AutomationSource は「記録が届いたとき」の予約で選べる記録の種類。
export type AutomationSource = { name: string; description: string }
export type Job = JobSpec & { id: string; next_run_at?: string; last_run_at?: string; last_status: string; last_error?: string }

export function listJobs(signal?: AbortSignal): Promise<{ jobs: Job[]; sources: AutomationSource[] }> {
  return requestJSON('/api/jobs', '予約ジョブを取得できませんでした', { signal })
}

export async function createJob(spec: JobSpec): Promise<void> {
  await requestJSON('/api/jobs', '予約ジョブを追加できませんでした', { method: 'POST', body: spec })
}

export async function updateJob(id: string, spec: JobSpec): Promise<void> {
  await requestJSON(`/api/jobs/${encodeURIComponent(id)}`, '予約ジョブを更新できませんでした', { method: 'PUT', body: spec })
}

export async function deleteJob(id: string): Promise<void> {
  await requestJSON(`/api/jobs/${encodeURIComponent(id)}`, '予約ジョブを削除できませんでした', { method: 'DELETE' })
}
