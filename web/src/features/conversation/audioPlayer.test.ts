import test from 'node:test'
import assert from 'node:assert/strict'
import { createAudioPlayer } from './audioPlayer.ts'

function setup() {
  const savedWindow = (globalThis as any).window
  const contexts: any[] = []
  class Context {
    state = 'running'; destination = {}; stream: any; closed = false
    gainNode = { gain: { value: 1 }, connect(_node: any) {}, disconnect() {} }
    sourceNode = { connect(_node: any) {}, disconnect() {} }
    constructor() { contexts.push(this) }
    createMediaStreamSource(stream: any) { this.stream = stream; return this.sourceNode }
    createGain() { return this.gainNode }
    async resume() {}
    async close() { this.closed = true }
  }
  ;(globalThis as any).window = { AudioContext: Context }
  return { contexts, restore() { (globalThis as any).window = savedWindow } }
}

test('WebRTC音声をWeb Audioで再生し、音量を変え、閉じたら解放する', () => {
  const { contexts, restore } = setup()
  const player = createAudioPlayer(1.5)
  const stream = {} as MediaStream
  try {
    player.attach(stream)
    assert.equal(contexts.length, 1)
    assert.equal(contexts[0].stream, stream)
    assert.equal(contexts[0].gainNode.gain.value, 1.5)
    player.volume(0.3)
    assert.equal(contexts[0].gainNode.gain.value, 0.3)
    player.volume(9)
    assert.equal(contexts[0].gainNode.gain.value, 1.5)
    player.volume(Number.NaN)
    assert.equal(contexts[0].gainNode.gain.value, 1.5)
    player.close()
    assert.equal(contexts[0].closed, true)
    player.attach(stream)
    assert.equal(contexts.length, 1)
  } finally { player.close(); restore() }
})

test('作る前に受け取った音量で再生を始める', () => {
  const { contexts, restore } = setup()
  const player = createAudioPlayer(0.6)
  try {
    player.attach({} as MediaStream)
    assert.equal(contexts[0].gainNode.gain.value, 0.6)
  } finally { player.close(); restore() }
})
