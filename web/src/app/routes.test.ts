import test from 'node:test'
import assert from 'node:assert/strict'
import { screenFromLocation, screenURL } from './routes.ts'

test('?ui= の値から画面を決める', () => {
  assert.equal(screenFromLocation('?ui=admin', '/'), 'eventlog')
  assert.equal(screenFromLocation('?ui=memory', '/'), 'memory')
  assert.equal(screenFromLocation('?ui=scenes', '/'), 'scenes')
  assert.equal(screenFromLocation('?ui=jobs', '/'), 'jobs')
  assert.equal(screenFromLocation('?ui=settings', '/'), 'settings')
  assert.equal(screenFromLocation('', '/'), 'live')
  assert.equal(screenFromLocation('?ui=unknown', '/'), 'live')
})

test('古いパスで開いた場合も受け付ける', () => {
  assert.equal(screenFromLocation('', '/admin'), 'eventlog')
  assert.equal(screenFromLocation('', '/jobs/'), 'jobs')
  assert.equal(screenFromLocation('?ui=unknown', '/memory'), 'memory')
})

test('画面の URL を作るとき ui 以外のクエリを残す', () => {
  assert.equal(screenURL('memory', ''), '/?ui=memory')
  assert.equal(screenURL('eventlog', '?debug=1&ui=jobs'), '/?debug=1&ui=admin')
  assert.equal(screenURL('live', '?ui=jobs'), '/')
  assert.equal(screenURL('live', '?debug=1&ui=jobs'), '/?debug=1')
})
