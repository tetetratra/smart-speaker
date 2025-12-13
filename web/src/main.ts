import { log } from './log'
import { createAudioReceiver, createAudioSender } from './audio'
import { createWS } from './ws'

const sendBtn = document.getElementById('send') as HTMLButtonElement | null
const connectBtn = document.getElementById('connect') as HTMLButtonElement | null
const disconnectBtn = document.getElementById('disconnect') as HTMLButtonElement | null

if (!sendBtn || !connectBtn || !disconnectBtn) {
  throw new Error('UI要素が見つかりません')
}

const ws = createWS('ws://localhost:8081/ws/audio')
const receiver = createAudioReceiver()
let senderStop: (() => void) | null = null

async function start() {
  await ws.connect((msg) => {
    if (msg?.type === 'audio.play' && msg.audio) {
      receiver.play(msg.audio)
    }
  })
  const sender = await createAudioSender((audioB64) => {
    ws.send({ type: 'audio.append', audio: audioB64 })
  })
  await sender.start()
  senderStop = () => sender.stop()
  log('接続しました')
}

function stop() {
  if (senderStop) {
    senderStop()
    senderStop = null
  }
  ws.close()
  log('切断しました')
}

connectBtn.addEventListener('click', () => {
  start().catch((e) => log('connect error: ' + (e as any)?.message))
})

disconnectBtn.addEventListener('click', () => {
  stop()
})

sendBtn.addEventListener('click', () => {
  // いまは常時送信しているので何もしない（プレースホルダ）
  log('送信開始（常時送信中）')
})
