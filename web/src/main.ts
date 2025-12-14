import { log } from './log'
import { createAudioReceiver, createAudioSender } from './audio'
import { createWS } from './ws'

const connectBtn = document.getElementById('connect') as HTMLButtonElement | null
const disconnectBtn = document.getElementById('disconnect') as HTMLButtonElement | null

if (!connectBtn || !disconnectBtn) {
  throw new Error('UI要素が見つかりません')
}

const ws = createWS('ws://localhost:8081/ws/audio')
const receiver = createAudioReceiver()
let senderStop: (() => void) | null = null
let lastSendLog = 0
let lastRecvLog = 0

async function start() {
  await ws.connect((msg) => {
    if (msg?.type === 'audio.play' && msg.audio) {
      receiver.play(msg.audio)
      const now = Date.now()
      if (now - lastRecvLog > 2000) {
        lastRecvLog = now
        const len = msg.audio.length
        log(`受信 audio.play len=${len} role=${msg.role ?? 'unknown'}`)
      }
    }
  })
  const sender = await createAudioSender((audioB64) => {
    ws.send({ type: 'audio.append', audio: audioB64 })
    const now = Date.now()
    if (now - lastSendLog > 2000) {
      lastSendLog = now
      log(`送信 audio.append len=${audioB64.length}`)
    }
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
