import test from 'node:test'
import assert from 'node:assert/strict'
import { WHITEBOARD_FIXED_SPEED, whiteboardScrollSpeed } from './whiteboardScroll.ts'

test('読み上げ区間の80%地点にちょうど末尾へ届く速さにする', () => {
  // 1000ms 時点で始まった 5000ms の区間なら、80%地点は 5000ms。2000ms 時点では残り 3000ms。
  assert.equal(whiteboardScrollSpeed(300, { startedAt: 1000, durationMs: 5000 }, 2000), 100)
})

test('読み上げがない、または80%地点が近すぎるときは決まった速さにする', () => {
  assert.equal(whiteboardScrollSpeed(300, undefined, 2000), WHITEBOARD_FIXED_SPEED)
  assert.equal(whiteboardScrollSpeed(300, { startedAt: 1000 }, 2000), WHITEBOARD_FIXED_SPEED)
  assert.equal(whiteboardScrollSpeed(300, { startedAt: 1000, durationMs: 1000 }, 2000), WHITEBOARD_FIXED_SPEED)
  assert.equal(whiteboardScrollSpeed(300, { startedAt: 1000, durationMs: 2000 }, 2500), WHITEBOARD_FIXED_SPEED)
})

test('動かす距離がないときは決まった速さを返す', () => {
  assert.equal(whiteboardScrollSpeed(0, { startedAt: 1000, durationMs: 5000 }, 2000), WHITEBOARD_FIXED_SPEED)
})
