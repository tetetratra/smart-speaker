import assert from 'node:assert/strict'
import test from 'node:test'
import { entryTone, historyEntries, whiteboardEvents, type HistoryEntry } from './history.ts'
import type { ConversationRecord } from '../conversation/protocol.ts'

const tool = (id: string, overrides: Record<string, unknown> = {}): HistoryEntry => ({
  kind: 'tool', id, role: 'tool', call_id: id, name: 'set_whiteboard', arguments: '{}',
  output: JSON.stringify({ content: `本文${id}`, updated: true }),
  started_at: '2026-11-11T00:00:00.000Z', ...overrides,
} as HistoryEntry)

test('同時刻の呼び出しも、それぞれの履歴の直後に本文を表示する', () => {
  assert.deepEqual(whiteboardEvents([tool('a'), tool('b')]).map(event => event.id), [
    'tool-a', 'board-a', 'tool-b', 'board-b',
  ])
})

test('再描画で本文が重複しない', () => {
  const entries = [tool('a')]
  const first = whiteboardEvents(entries)
  assert.deepEqual(whiteboardEvents(entries), first)
  assert.equal(first.filter(event => event.kind === 'board').length, 1)
  assert.equal(entries.length, 1)
})

test('失敗・他ツール・不正な結果・空本文からは本文を表示しない', () => {
  const entries = [
    tool('failed', { error: '失敗' }), tool('other', { name: 'web_search' }),
    tool('invalid', { output: '{' }), tool('null', { output: 'null' }),
    tool('empty', { output: '{"content":"  "}' }),
    { kind: 'inbound', id: 'inbound', role: 'system', source: '外部', name: '外部', text: '通知', started_at: '2026-11-11T00:00:00Z' } as HistoryEntry,
  ]
  assert.equal(whiteboardEvents(entries).length, entries.length)
  assert.ok(whiteboardEvents(entries).every(event => event.kind === 'history'))
})

test('記録の受信順によらず時刻順に表示する', () => {
  assert.deepEqual(whiteboardEvents([tool('new', { started_at: '2026-11-11T00:01:00Z' }), tool('old')]).map(event => event.id), [
    'tool-old', 'board-old', 'tool-new', 'board-new',
  ])
})

const record = (id: string, overrides: Partial<ConversationRecord> = {}): ConversationRecord => ({
  id, role: 'tool', text: '', source: 'realtime', created_at: '2026-11-11T00:00:00.000Z', ...overrides,
})

test('ツールの記録は呼び出しと結果を1件で持ち、記録の error で成功・失敗を決める', () => {
  const records = [
    record('tool-a', { source: 'set_whiteboard', call_id: 'a', title: 'メモ', arguments: '{"content":"本文"}', output: '{"content":"本文"}' }),
    record('tool-b', { source: 'web_search', call_id: 'b', output: '{"error":"失敗"}', error: '失敗' }),
  ]
  const entries = historyEntries(records)
  assert.deepEqual(entries.map(entry => entry.kind === 'tool' && [entry.id, entryTone(entry)]), [
    ['tool-a', 'tool'], ['tool-b', 'failed'],
  ])
  const first = entries[0]
  assert.ok(first.kind === 'tool' && first.title === 'メモ' && first.arguments === '{"content":"本文"}' && first.output === '{"content":"本文"}')
  assert.deepEqual(whiteboardEvents(entries).map(event => event.id), ['tool-tool-a', 'board-tool-a', 'tool-tool-b'])
})

test('後から届いた結果や通知は差し込みとして出し、見出しの名前は source をそのまま出す', () => {
  const entries = historyEntries([
    record('search', { role: 'background', source: 'web_search', title: '天気', text: '検索結果', external: true }),
    record('notice', { role: 'background', source: 'claude_code', title: 'READMEを直して', text: '通知' }),
  ])
  assert.deepEqual(entries.map(entry => [entry.kind, entry.name, entry.title]), [
    ['inbound', 'web_search', '天気'], ['inbound', 'claude_code', 'READMEを直して'],
  ])
})

test('色は Role で決め、error があれば Role によらず赤にする', () => {
  const entries = historyEntries([
    record('tool'),
    record('search', { role: 'background', text: '検索結果' }),
    record('remind', { role: 'system', text: '指示' }),
    record('job-failed', { role: 'system', text: '失敗', error: '例外' }),
    record('claude-failed', { role: 'background', text: '失敗', error: '中止' }),
  ])
  assert.deepEqual(entries.map(entryTone), ['tool', 'background', 'system', 'failed', 'failed'])
})
