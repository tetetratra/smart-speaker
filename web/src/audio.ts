const SAMPLE_RATE = 24000

export interface AudioReceiver {
  play: (b64pcm: string) => void
  stop: () => void
}

export function createAudioReceiver(): AudioReceiver {
  const ctx = new AudioContext()
  let playhead = 0
  const sources = new Set<AudioBufferSourceNode>()
  return {
    play: (b64pcm: string) => {
      try {
        const pcm = base64ToInt16(b64pcm)
        const audioBuf = int16ToAudioBuffer(ctx, pcm, SAMPLE_RATE)
        const src = ctx.createBufferSource()
        src.buffer = audioBuf
        src.connect(ctx.destination)
        sources.add(src)
        src.onended = () => {
          sources.delete(src)
        }
        const startAt = Math.max(ctx.currentTime, playhead)
        src.start(startAt)
        playhead = startAt + audioBuf.duration
      } catch (e) {
        console.warn('audio play error:', (e as any)?.message)
      }
    },
    stop: () => {
      sources.forEach((src) => {
        try {
          src.stop()
        } catch {}
      })
      sources.clear()
      playhead = ctx.currentTime
    },
  }
}

function base64ToInt16(b64: string): Int16Array {
  const binary = atob(b64)
  const buf = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++) {
    buf[i] = binary.charCodeAt(i)
  }
  return new Int16Array(buf.buffer)
}

function int16ToAudioBuffer(ctx: AudioContext, data: Int16Array, sampleRate: number): AudioBuffer {
  const audioBuffer = ctx.createBuffer(1, data.length, sampleRate)
  const channel = audioBuffer.getChannelData(0)
  for (let i = 0; i < data.length; i++) {
    channel[i] = data[i] / 0x7fff
  }
  return audioBuffer
}
