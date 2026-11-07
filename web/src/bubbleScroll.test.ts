import test from 'node:test'
import assert from 'node:assert/strict'
import { bubbleScrollTop } from './bubbleScroll.ts'

test('再生の経過時間に比例してスクロールし、再生終了時にちょうど末尾へ到達する', () => {
  assert.equal(bubbleScrollTop(0, 4000, 200), 0)
  assert.equal(bubbleScrollTop(1000, 4000, 200), 50)
  assert.equal(bubbleScrollTop(4000, 4000, 200), 200)
  assert.equal(bubbleScrollTop(6000, 4000, 200), 200)
})

test('吹き出しに収まる場合や再生時間が不明な場合はスクロールしない', () => {
  assert.equal(bubbleScrollTop(1000, 4000, 0), 0)
  assert.equal(bubbleScrollTop(1000, 0, 200), 0)
  assert.equal(bubbleScrollTop(1000, Number.NaN, 200), 0)
})
