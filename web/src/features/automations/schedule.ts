import type { Job, Schedule } from './api.ts'

const weekdays = ['日', '月', '火', '水', '木', '金', '土']
const pad = (value: number) => String(value).padStart(2, '0')

// formatDateTime は日時を「11/23(日) 8:05」の形にする。読めなければそのまま返す。
export function formatDateTime(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return `${date.getMonth() + 1}/${date.getDate()}(${weekdays[date.getDay()]}) ${date.getHours()}:${pad(date.getMinutes())}`
}

// describeCron はよく使う形の cron だけ読みやすくし、それ以外は式をそのまま返す。
export function describeCron(cron: string): string {
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

// describeSchedule は一覧に出す予約の説明を返す。
export function describeSchedule(job: Job): string {
  if (job.schedule.cron) return describeCron(job.schedule.cron)
  if (job.schedule.source) return `記録: ${job.schedule.source}`
  if (job.schedule.at) return formatDateTime(job.schedule.at)
  return ''
}

export type ScheduleKind = 'cron' | 'at' | 'source'

// scheduleKind は編集画面で選ぶ予約の種類を返す。
export function scheduleKind(schedule: Schedule): ScheduleKind {
  if (schedule.cron !== undefined) return 'cron'
  if (schedule.source !== undefined) return 'source'
  return 'at'
}
