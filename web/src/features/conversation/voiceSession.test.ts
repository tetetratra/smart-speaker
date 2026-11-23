import test from 'node:test'
import assert from 'node:assert/strict'
import { createVoiceSession } from './voiceSession.ts'
import type { SessionEvent } from './sessionReducer.ts'

// WebSocket・RTCPeerConnection・マイク・タイマーを差し替え、音声接続の出来事の順番を確かめる。
function setup() {
  const saved = { window: (globalThis as any).window, WebSocket: (globalThis as any).WebSocket, RTCPeerConnection: (globalThis as any).RTCPeerConnection, navigator: Object.getOwnPropertyDescriptor(globalThis, 'navigator') }
  const sockets: FakeSocket[] = []
  const timers: { fn: () => void; delay: number }[] = []
  const microphones: { resolve: (stream: any) => void }[] = []
  const stoppedTracks: number[] = []
  class FakeSocket {
    static OPEN = 1
    readyState = 0
    sent: unknown[] = []
    onopen: (() => void) | null = null
    onmessage: ((event: { data: unknown }) => void) | null = null
    onclose: (() => void) | null = null
    url: string
    constructor(url: string) { this.url = url; sockets.push(this) }
    send(data: string) { this.sent.push(JSON.parse(data)) }
    close() { this.readyState = 3; this.onclose?.() }
    open() { this.readyState = 1; this.onopen?.() }
    receive(message: unknown) { this.onmessage?.({ data: JSON.stringify(message) }) }
  }
  class FakePeer {
    onicecandidate: unknown = null
    ontrack: unknown = null
    closed = false
    addTrack() {}
    async createOffer() { return { type: 'offer', sdp: 'offer-sdp' } }
    async setLocalDescription() {}
    close() { this.closed = true }
  }
  ;(globalThis as any).window = {
    location: { origin: 'http://localhost:8081' },
    setTimeout: (fn: () => void, delay: number) => timers.push({ fn, delay }),
    clearTimeout: () => {},
  }
  ;(globalThis as any).WebSocket = FakeSocket
  ;(globalThis as any).RTCPeerConnection = FakePeer
  Object.defineProperty(globalThis, 'navigator', {
    configurable: true,
    value: { mediaDevices: { getUserMedia: () => new Promise(resolve => microphones.push({ resolve })) } },
  })
  const stream = (id: number) => ({ getTracks: () => [{ stop: () => stoppedTracks.push(id) }], getAudioTracks: () => [{}] })
  return {
    sockets, timers, microphones, stoppedTracks, stream,
    restore() {
      ;(globalThis as any).window = saved.window
      ;(globalThis as any).WebSocket = saved.WebSocket
      ;(globalThis as any).RTCPeerConnection = saved.RTCPeerConnection
      if (saved.navigator) Object.defineProperty(globalThis, 'navigator', saved.navigator)
    },
  }
}

const flush = () => new Promise(resolve => setTimeout(resolve, 0))

test('開いたらマイクを取って offer を送り、届いたメッセージを出来事とログに渡す', async () => {
  const env = setup()
  const events: string[] = []
  const raw: unknown[] = []
  try {
    const session = createVoiceSession({ initialVolume: 1, onEvent: (event: SessionEvent) => events.push(event.type), onRawMessage: (message) => raw.push(message) })
    session.connect()
    assert.equal(env.sockets[0].url, 'ws://localhost:8081/ws/chat')
    env.sockets[0].open()
    env.microphones[0].resolve(env.stream(1))
    await flush()
    assert.deepEqual(env.sockets[0].sent, [{ type: 'webrtc.offer', sdp: 'offer-sdp' }])
    env.sockets[0].receive({ type: 'speech_start' })
    env.sockets[0].receive({ type: 'webrtc.ice', candidate: { candidate: 'x' } })
    assert.deepEqual(events, ['connecting', 'open', 'mic_ready', 'message'])
    assert.equal(raw.length, 2)
    session.disconnect()
  } finally { env.restore() }
})

test('閉じたら 1 秒から倍々で 10 回までつなぎ直す', () => {
  const env = setup()
  const events: string[] = []
  try {
    const session = createVoiceSession({ initialVolume: 1, onEvent: (event) => events.push(event.type), onRawMessage: () => {} })
    session.connect()
    for (let i = 0; i < 11; i++) {
      env.sockets.at(-1)!.close()
      if (env.timers[i]) env.timers[i].fn()
    }
    assert.deepEqual(env.timers.map(timer => timer.delay), [1000, 2000, 4000, 8000, 16000, 32000, 64000, 128000, 256000, 512000])
    assert.equal(env.sockets.length, 11)
    // 諦めた後に connect を呼んだら、すぐ接続し直す。
    session.connect()
    assert.equal(env.sockets.length, 12)
    assert.deepEqual(events.slice(-1), ['connecting'])
  } finally { env.restore() }
})

test('切断した後に取れたマイクは止め、出来事を出さない。disconnect は何度呼んでもよい', async () => {
  const env = setup()
  const events: string[] = []
  try {
    const session = createVoiceSession({ initialVolume: 1, onEvent: (event) => events.push(event.type), onRawMessage: () => {} })
    session.connect()
    env.sockets[0].open()
    session.disconnect()
    session.disconnect()
    env.microphones[0].resolve(env.stream(7))
    await flush()
    assert.deepEqual(env.stoppedTracks, [7])
    assert.deepEqual(events, ['connecting', 'open', 'disconnected', 'disconnected'])
    // 切断した後は、つなぎ直さない。
    assert.equal(env.timers.length, 0)
  } finally { env.restore() }
})

test('接続し直した後に、前の接続のマイクが取れても無視する', async () => {
  const env = setup()
  const events: string[] = []
  try {
    const session = createVoiceSession({ initialVolume: 1, onEvent: (event) => events.push(event.type), onRawMessage: () => {} })
    session.connect()
    env.sockets[0].open()
    session.connect()
    env.sockets[1].open()
    env.microphones[0].resolve(env.stream(1))
    env.microphones[1].resolve(env.stream(2))
    await flush()
    assert.deepEqual(env.stoppedTracks, [1])
    assert.deepEqual(events, ['connecting', 'open', 'connecting', 'open', 'mic_ready'])
    assert.deepEqual(env.sockets[0].sent, [])
    session.disconnect()
  } finally { env.restore() }
})
