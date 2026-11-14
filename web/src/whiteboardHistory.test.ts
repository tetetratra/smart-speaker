import assert from 'node:assert/strict'
import test from 'node:test'
import { historyEntries, mergeRecords, whiteboardEvents, type HistoryEntry, type WhiteboardRecord } from './whiteboardHistory.ts'

const tool = (id: string, overrides: Record<string, unknown> = {}): HistoryEntry => ({
  kind: 'tool', id, call_id: id, name: 'set_whiteboard', arguments: '{}',
  output: JSON.stringify({ content: `本文${id}`, updated: true }), status: 'succeeded',
  started_at: '2026-11-11T00:00:00.000Z', ...overrides,
} as HistoryEntry)

test('同時刻の呼び出しも、それぞれの履歴の直後に本文を表示する', () => {
  assert.deepEqual(whiteboardEvents([tool('a'), tool('b')]).map(event => event.id), [
    'tool-a', 'board-a', 'tool-b', 'board-b',
  ])
})

test('開始から完了への更新と再描画で本文が重複しない', () => {
  assert.deepEqual(whiteboardEvents([tool('a', { status: 'running' })]).map(event => event.kind), ['history'])
  const entries = [tool('a')]
  const first = whiteboardEvents(entries)
  assert.deepEqual(whiteboardEvents(entries), first)
  assert.equal(first.filter(event => event.kind === 'board').length, 1)
  assert.equal(entries.length, 1)
})

test('失敗・他ツール・不正な結果・空本文からは本文を表示しない', () => {
  const entries = [
    tool('failed', { status: 'failed' }), tool('other', { name: 'web_search' }),
    tool('invalid', { output: '{' }), tool('null', { output: 'null' }),
    tool('empty', { output: '{"content":"  "}' }),
    { kind: 'inbound', id: 'inbound', source: '外部', name: '外部', text: '通知', started_at: '2026-11-11T00:00:00Z' } as HistoryEntry,
  ]
  assert.equal(whiteboardEvents(entries).length, entries.length)
  assert.ok(whiteboardEvents(entries).every(event => event.kind === 'history'))
})

test('記録の受信順によらず時刻順に表示する', () => {
  assert.deepEqual(whiteboardEvents([tool('new', { started_at: '2026-11-11T00:01:00Z' }), tool('old')]).map(event => event.id), [
    'tool-old', 'board-old', 'tool-new', 'board-new',
  ])
})

const record = (id: string, overrides: Partial<WhiteboardRecord> = {}): WhiteboardRecord => ({
  id, role: 'tool_call', text: '', source: 'realtime', created_at: '2026-11-11T00:00:00.000Z', ...overrides,
})

test('呼び出しと結果の記録を call_id で組み合わせ、実行中・完了・失敗を決める', () => {
  const records = [
    record('call-a', { tool_name: 'set_whiteboard', call_id: 'a', title: 'メモ', arguments: '{"content":"本文"}' }),
    record('result-a', { role: 'tool_result', tool_name: 'set_whiteboard', call_id: 'a', output: '{"content":"本文"}', created_at: '2026-11-11T00:00:01.000Z' }),
    record('call-b', { tool_name: 'web_search', call_id: 'b' }),
    record('call-c', { tool_name: 'web_search', call_id: 'c' }),
    record('result-c', { role: 'tool_result', tool_name: 'web_search', call_id: 'c', output: '{"error":"失敗"}' }),
  ]
  const entries = historyEntries(records)
  assert.deepEqual(entries.map(entry => entry.kind === 'tool' && [entry.id, entry.status]), [
    ['call-a', 'succeeded'], ['call-b', 'running'], ['call-c', 'failed'],
  ])
  const first = entries[0]
  assert.ok(first.kind === 'tool' && first.title === 'メモ' && first.output === '{"content":"本文"}' && first.finished_at === '2026-11-11T00:00:01.000Z')
  assert.deepEqual(whiteboardEvents(entries).map(event => event.id), ['tool-call-a', 'board-call-a', 'tool-call-b', 'tool-call-c'])
})

test('呼び出しと対にならない結果や通知は差し込みとして出す', () => {
  const entries = historyEntries([
    record('search', { role: 'tool_result', source: 'tool/web_search', title: '天気', text: '検索結果', external: true }),
    record('notice', { role: 'system', source: 'agent/claude-code/stop', title: '完了', text: '通知' }),
  ])
  assert.deepEqual(entries.map(entry => [entry.kind, entry.name, entry.title]), [
    ['inbound', 'tool/web_search', '天気'], ['inbound', 'agent/claude-code/stop', '完了'],
  ])
})

test('差分は ID で上書きし、順番が前後しても重複しない', () => {
  let merged = mergeRecords(new Map(), [record('result', { role: 'tool_result', call_id: 'a' })])
  merged = mergeRecords(merged, [record('call', { call_id: 'a' }), record('result', { role: 'tool_result', call_id: 'a' })])
  assert.equal(merged.size, 2)
  assert.deepEqual(historyEntries(merged.values()).map(entry => entry.id), ['call'])
})
