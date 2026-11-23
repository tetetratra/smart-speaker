import test from 'node:test'
import assert from 'node:assert/strict'
import { formatServerEventLog } from './format.ts'

test('type を先頭にした 1 行の JSON にする', () => {
  assert.equal(formatServerEventLog({ generation_id: 3, type: 'playback_stop' }), '{"type":"playback_stop","generation_id":3}')
})

test('音声などの大きな値を「...」に置き換える', () => {
  const base64 = 'A'.repeat(256)
  assert.equal(
    formatServerEventLog({ type: 'x', audio: 'short', text: base64, nested: { pcm: [1, 2], list: [] }, buffer: new Uint8Array(2) }),
    '{"type":"x","audio":"...","text":"...","nested":{"pcm":"...","list":[]},"buffer":"..."}',
  )
})

test('JSON にできない値でも 1 行を返す', () => {
  assert.equal(formatServerEventLog({ type: 'big', value: BigInt(1) }), '{"type":"unserializable","value":"..."}')
})
