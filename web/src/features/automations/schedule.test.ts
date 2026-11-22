import test from 'node:test'
import assert from 'node:assert/strict'
import { describeCron, describeSchedule, formatDateTime, scheduleKind } from './schedule.ts'
import type { Job } from './api.ts'

test('よく使う形の cron を読みやすくする', () => {
  assert.equal(describeCron('0 8 * * *'), '毎日 8:00')
  assert.equal(describeCron('5 7 * * 1-5'), '平日 7:05')
  assert.equal(describeCron('30 9 * * 6,0'), '土日 9:30')
  assert.equal(describeCron('0 21 * * 1,3,7'), '毎週月・水・日 21:00')
})

test('読みやすくできない cron は式のまま返す', () => {
  assert.equal(describeCron('*/5 * * * *'), '*/5 * * * *')
  assert.equal(describeCron('0 8 1 * *'), '0 8 1 * *')
  assert.equal(describeCron('0 8 * *'), '0 8 * *')
})

test('日時を月日・曜日・時刻にする', () => {
  // 2026-11-23 は月曜日。端末のタイムゾーンによらないよう、ローカル時刻で作る。
  assert.equal(formatDateTime(new Date(2026, 10, 23, 8, 5).toISOString()), '11/23(月) 8:05')
  assert.equal(formatDateTime('not a date'), 'not a date')
})

test('予約の種類と説明を決める', () => {
  const job = (schedule: Job['schedule']): Job => ({ id: 'j', name: 'n', script: '', schedule, last_status: '' })
  assert.equal(describeSchedule(job({ cron: '0 8 * * *' })), '毎日 8:00')
  assert.equal(describeSchedule(job({ source: 'temperature' })), '記録: temperature')
  assert.equal(scheduleKind({ cron: '' }), 'cron')
  assert.equal(scheduleKind({ source: '' }), 'source')
  assert.equal(scheduleKind({ at: '' }), 'at')
  assert.equal(scheduleKind({}), 'at')
})
