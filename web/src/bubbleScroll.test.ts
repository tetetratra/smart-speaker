import test from 'node:test'
import assert from 'node:assert/strict'
import { translateYFromTransform } from './bubbleScroll.ts'

test('アニメーション途中のtransformから縦方向の移動量を取り出す', () => {
  assert.equal(translateYFromTransform('matrix(1, 0, 0, 1, 0, -37.5)'), -37.5)
  assert.equal(translateYFromTransform('matrix3d(1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, -12.25, 0, 1)'), -12.25)
})

test('transformが無い場合や読めない場合は移動量を0とみなす', () => {
  assert.equal(translateYFromTransform('none'), 0)
  assert.equal(translateYFromTransform(''), 0)
  assert.equal(translateYFromTransform('matrix(1, 0, 0, 1, 0, abc)'), 0)
})
