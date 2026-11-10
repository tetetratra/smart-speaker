import assert from 'node:assert/strict'
import test from 'node:test'
import { whiteboardEvents, type HistoryEntry } from './whiteboardHistory.ts'

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

test('開始から完了への更新とスナップショットの再送で本文が重複しない', () => {
  assert.deepEqual(whiteboardEvents([tool('a', { status: 'running' })]).map(event => event.kind), ['history'])
  const snapshot = [tool('a')]
  const first = whiteboardEvents(snapshot)
  assert.deepEqual(whiteboardEvents(snapshot), first)
  assert.equal(first.filter(event => event.kind === 'board').length, 1)
  assert.equal(snapshot.length, 1)
})

test('失敗・他ツール・不正な結果・空本文からは本文を表示しない', () => {
  const entries = [
    tool('failed', { status: 'failed' }), tool('other', { name: 'web_search' }),
    tool('invalid', { output: '{' }), tool('null', { output: 'null' }),
    tool('empty', { output: '{"content":"  "}' }),
    { kind: 'inbound', id: 'inbound', source: '外部', text: '通知', started_at: '2026-11-11T00:00:00Z' } as HistoryEntry,
  ]
  assert.equal(whiteboardEvents(entries).length, entries.length)
  assert.ok(whiteboardEvents(entries).every(event => event.kind === 'history'))
})

test('新しい順のスナップショットを時刻順に復元する', () => {
  assert.deepEqual(whiteboardEvents([tool('new', { started_at: '2026-11-11T00:01:00Z' }), tool('old')]).map(event => event.id), [
    'tool-old', 'board-old', 'tool-new', 'board-new',
  ])
})
