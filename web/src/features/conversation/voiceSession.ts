import { wsURL } from '../../shared/backend'
import { createSocket, type Socket } from '../../shared/socket'
import { createAudioPlayer, type AudioPlayer } from './audioPlayer'
import { createPeer, type Peer } from './peer'
import { isWebRTCSignal } from './protocol'
import type { SessionEvent } from './sessionReducer'

// 自動でつなぎ直すのは 10 回まで。待ち時間は 1 秒から倍々に延ばす。
const reconnectMaxAttempts = 10
const reconnectInitialDelayMs = 1000

export type VoiceSession = {
  // 接続する。自動再接続を待っている間や諦めた後に呼んだら、回数を数え直してすぐ接続し直す。
  connect(): void
  // 切断し、自動再接続もやめる。何度呼んでもよく、その後また connect できる。
  disconnect(): void
  // 再生音量（倍率）を変える。接続し直しても引き継ぐ。
  setVolume(gain: number): void
}

// createVoiceSession は音声接続の I/O 全体（/ws/chat のソケット・WebRTC・音声の再生）を持つ。React には依存しない。
// 画面の状態に関わる出来事とメッセージは onEvent に、受け取ったメッセージはすべて加工前のまま onRawMessage にも渡す（ログに出すため）。
export function createVoiceSession(options: {
  initialVolume: number
  onEvent: (event: SessionEvent) => void
  onRawMessage: (message: unknown) => void
}): VoiceSession {
  const { onEvent, onRawMessage } = options
  let volume = options.initialVolume
  let socket: Socket | null = null
  let peer: Peer | null = null
  let player: AudioPlayer | null = null

  const stopPeer = () => {
    peer?.stop()
    peer = null
    player?.close()
    player = null
  }
  // マイクの取得に失敗しても、ソケットは開いたままにする。閉じると、自動再接続のたびにマイクの許可を求め直すことになるため。
  const startPeer = (current: Socket) => {
    const currentPlayer = createAudioPlayer(volume, (message) => console.error(message))
    const currentPeer = createPeer((message) => current.send(message), (stream) => currentPlayer.attach(stream))
    player = currentPlayer
    peer = currentPeer
    void currentPeer.start().then((ok) => {
      if (peer !== currentPeer) return
      if (ok) {
        onEvent({ type: 'mic_ready' })
        return
      }
      stopPeer()
      onEvent({ type: 'mic_failed' })
    })
  }

  return {
    connect() {
      socket?.close()
      stopPeer()
      const current: Socket = createSocket(wsURL('/ws/chat'), {
        onConnecting: () => onEvent({ type: 'connecting' }),
        onOpen: () => {
          onEvent({ type: 'open' })
          startPeer(current)
        },
        onMessage: (message) => {
          onRawMessage(message)
          if (message && typeof message === 'object' && isWebRTCSignal(message)) {
            void peer?.handleSignal(message)
            return
          }
          onEvent({ type: 'message', message })
        },
        onClose: () => {
          stopPeer()
          onEvent({ type: 'closed' })
        },
        retryDelay: (attempt) => {
          if (attempt > reconnectMaxAttempts) return undefined
          const delay = reconnectInitialDelayMs * 2 ** (attempt - 1)
          console.log(`[ws reconnect] attempt ${attempt}/${reconnectMaxAttempts} in ${delay}ms`)
          return delay
        },
      })
      socket = current
    },
    disconnect() {
      socket?.close()
      socket = null
      stopPeer()
      onEvent({ type: 'disconnected' })
    },
    setVolume(gain) {
      volume = gain
      player?.volume(gain)
    },
  }
}
