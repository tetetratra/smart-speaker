import test from 'node:test'
import assert from 'node:assert/strict'
import { GenerationQueue, createAudioReceiver } from './audio.ts'

test('新世代で待機音声を破棄し、空になった後も古い世代を拒否する', () => {
 const queue = new GenerationQueue<{generation_id:number;value:string}>()
 queue.push({generation_id:1,value:'旧'})
 queue.push({generation_id:2,value:'新'})
 assert.equal(queue.shift()?.value,'新')
 assert.equal(queue.push({generation_id:1,value:'後着の旧'}),false)
 assert.equal(queue.advance(1),false)
 assert.equal(queue.advance(2),false)
 assert.equal(queue.shift(),undefined)
})

test('停止通知と音声が逆順でも新音声を止めず、新世代通知では予約済み音声も止める', () => {
 const made: any[] = []
 class AudioContextStub {
  currentTime=0;state='running';destination={}
  createGain(){return {gain:{value:1},connect(){},disconnect(){}}}
  createBuffer(_:number,length:number,rate:number){return {duration:length/rate,getChannelData(){return new Float32Array(length)}}}
  createBufferSource(){const source={onended:null,buffer:null,stopped:false,connect(){},disconnect(){},start(){},stop(){this.stopped=true}};made.push(source);return source}
  async resume(){} async close(){}
 }
 ;(globalThis as any).AudioContext=AudioContextStub
 ;(globalThis as any).window=globalThis
 const receiver=createAudioReceiver()
 const frame={generation_id:2,segment_id:'x',sequence:0,audio:'AAA=',sample_rate:24000,final:true}
 receiver.play(frame)
 assert.equal(made.length,1)
 receiver.stop(2)
 assert.equal(made[0].stopped,false)
 receiver.stop(1)
 assert.equal(made[0].stopped,false)
 receiver.stop(3)
 assert.equal(made[0].stopped,true)
 receiver.play(frame)
 assert.equal(made.length,1)
 receiver.play({...frame,generation_id:3})
 assert.equal(made.length,2)
 receiver.close()
 assert.equal(made[1].stopped,true)
})
