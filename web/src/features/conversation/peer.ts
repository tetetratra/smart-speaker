import type { WebRTCSignalMessage } from './protocol.ts'

// WebRTC の接続。マイクを取ってサーバーへ送り、サーバーからの読み上げ音声を受け取る。
// offer・answer・ICE の候補は、/ws/chat のソケットを通してやり取りする。
export type Peer = {
  // マイクを取り、offer を送る。始められたら true を返す。途中で stop されたら false を返す。
  start(): Promise<boolean>
  // サーバーから届いた answer と ICE の候補を反映する。
  handleSignal(message: WebRTCSignalMessage): Promise<void>
  stop(): void
}

export function createPeer(send: (message: unknown) => void, onTrack: (stream: MediaStream) => void): Peer {
  const connection = new RTCPeerConnection()
  let microphone: MediaStream | null = null
  // answer（remoteDescription）より先に届いた ICE の候補は、answer を反映してから足す。
  let pendingICE: RTCIceCandidateInit[] = []
  let stopped = false

  connection.onicecandidate = (event) => {
    if (event.candidate) send({ type: 'webrtc.ice', candidate: event.candidate.toJSON() })
  }
  connection.ontrack = (event) => {
    if (stopped || event.track.kind !== 'audio') return
    onTrack(event.streams[0] ?? new MediaStream([event.track]))
  }

  const stop = () => {
    if (stopped) return
    stopped = true
    connection.onicecandidate = null
    connection.ontrack = null
    connection.close()
    microphone?.getTracks().forEach((track) => track.stop())
    microphone = null
    pendingICE = []
  }

  return {
    async start() {
      try {
        const stream = await navigator.mediaDevices.getUserMedia({
          audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true },
        })
        if (stopped) {
          stream.getTracks().forEach((track) => track.stop())
          return false
        }
        microphone = stream
        const track = stream.getAudioTracks()[0]
        if (!track) throw new Error('マイクトラックを取得できませんでした')
        connection.addTrack(track, stream)
        const offer = await connection.createOffer()
        await connection.setLocalDescription(offer)
        if (stopped) return false
        send({ type: 'webrtc.offer', sdp: offer.sdp })
        return true
      } catch (err) {
        if (!stopped) console.error('WebRTC を始められませんでした', err)
        stop()
        return false
      }
    },
    async handleSignal(message) {
      if (stopped) return
      try {
        if (message.type === 'webrtc.answer') {
          if (!message.sdp) return
          await connection.setRemoteDescription({ type: 'answer', sdp: String(message.sdp) })
          const pending = pendingICE
          pendingICE = []
          for (const candidate of pending) await connection.addIceCandidate(candidate)
          return
        }
        if (!message.candidate) return
        if (!connection.remoteDescription) {
          pendingICE.push(message.candidate)
          return
        }
        await connection.addIceCandidate(message.candidate)
      } catch (err) {
        console.error(`WebRTC の ${message.type} を反映できませんでした`, err)
      }
    },
    stop,
  }
}
