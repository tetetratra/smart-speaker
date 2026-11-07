import test from 'node:test'
import assert from 'node:assert/strict'
import { createAudioReceiver } from './audio.ts'

function setup() {
 const savedWindow = (globalThis as any).window
 const contexts: any[] = []
 class Context {
  state = 'running'; destination = {}; stream: any; closed = false
  gainNode = {gain: {value: 1}, connect(_node: any) {}, disconnect() {}}
  sourceNode = {connect(_node: any) {}, disconnect() {}}
  constructor() {contexts.push(this)}
  createMediaStreamSource(stream: any) {this.stream = stream; return this.sourceNode}
  createGain() {return this.gainNode}
  async resume() {} async close() {this.closed = true}
 }
 ;(globalThis as any).window = {AudioContext: Context}
 return {contexts, restore() {(globalThis as any).window = savedWindow}}
}

test('WebRTC音声をWeb Audioで再生し、世代変更では接続を維持して切断時に解放する', () => {
 const {contexts, restore} = setup()
 const shown: string[] = []
 const playing: boolean[] = []
 const receiver = createAudioReceiver(value => playing.push(value), speech => shown.push(speech.text))
 const stream = {} as MediaStream
 try {
  receiver.volume(1.5)
  receiver.attach(stream)
  assert.equal(contexts.length, 1)
  assert.equal(contexts[0].stream, stream)
  assert.equal(contexts[0].gainNode.gain.value, 1.5)
  receiver.volume(0.3)
  assert.equal(contexts[0].gainNode.gain.value, 0.3)
  receiver.progress({generation_id:1,segment_id:'old',text:'旧応答'})
  receiver.stop(2)
  assert.equal(contexts[0].closed, false)
  receiver.progress({generation_id:1,segment_id:'late',text:'遅れた旧応答'})
  receiver.progress({generation_id:2,segment_id:'new',text:'新応答'})
  receiver.stop(1)
  receiver.stop(2)
  receiver.progress({generation_id:1,segment_id:'',end:true})
  assert.deepEqual(shown, ['旧応答','新応答'])
  assert.deepEqual(playing, [true,false,true])
  receiver.progress({generation_id:2,segment_id:'',end:true})
  assert.equal(playing.at(-1), false)
  receiver.close()
  assert.equal(contexts[0].closed, true)
  receiver.attach(stream)
  receiver.progress({generation_id:3,segment_id:'late',text:'切断後'})
  assert.equal(contexts.length, 1)
  assert.equal(shown.length, 2)
 } finally {receiver.close();restore()}
})
