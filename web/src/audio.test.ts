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

test('読み上げ区間は再生開始時だけ通知し、先読み・末尾・停止後の旧タイマーでは表示を進めない', () => {
 const savedAudioContext=(globalThis as any).AudioContext
 const savedWindow=(globalThis as any).window
 const timeouts=new Map<number,()=>void>()
 let timerID=0
 let tick=()=>{}
 let context: any
 class AudioContextStub {
  currentTime=0;state='running';destination={}
  constructor(){context=this}
  createGain(){return {gain:{value:1},connect(){},disconnect(){}}}
  createBuffer(_:number,length:number,rate:number){return {duration:length/rate,getChannelData(){return new Float32Array(length)}}}
  createBufferSource(){return {buffer:null,onended:null,connect(){},disconnect(){},start(){},stop(){}}}
  async resume(){} async close(){}
 }
 ;(globalThis as any).AudioContext=AudioContextStub
 ;(globalThis as any).window={
  setInterval(callback:()=>void){tick=callback;return 0},clearInterval(){},
  setTimeout(callback:()=>void){const id=++timerID;timeouts.set(id,callback);return id},
  clearTimeout(id:number){timeouts.delete(id)},
 }
 const shown: string[]=[]
 const receiver=createAudioReceiver(()=>{},speech=>shown.push(speech.text))
 const flush=()=>{for(const [id,callback] of [...timeouts]){timeouts.delete(id);callback()}}
 const frame={generation_id:1,segment_id:'first',sequence:0,audio:btoa('\0'.repeat(4800)),sample_rate:24000,final:false,text:'最初の区間。'}
 try {
  receiver.play(frame)
  assert.deepEqual(shown,[])
  receiver.play({...frame,sequence:1,final:true})
  receiver.play({...frame,segment_id:'next',text:'次の区間。'})
  flush()
  assert.deepEqual(shown,['最初の区間。'])
  context.currentTime=1
  receiver.play({...frame,segment_id:'last',text:'最後の区間。'})
  assert.equal(shown.length,1)
  flush()
  assert.equal(shown.length,1)
  context.currentTime=1.2
  tick()
  flush()
  assert.deepEqual(shown,['最初の区間。','次の区間。'])
  context.currentTime=1.4
  tick()
  receiver.stop(2)
  flush()
  assert.equal(shown.length,2)
  receiver.play({...frame,generation_id:1,text:'遅延した旧音声'})
  flush()
  assert.equal(shown.length,2)
  receiver.play({...frame,generation_id:2,text:'新しい区間。'})
  receiver.close()
  flush()
  assert.equal(shown.length,2)
 } finally {
  receiver.close()
  ;(globalThis as any).AudioContext=savedAudioContext
  ;(globalThis as any).window=savedWindow
 }
})
