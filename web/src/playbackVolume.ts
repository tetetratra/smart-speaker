import { useCallback, useEffect, useRef, useState } from 'react'

// サーバーの internal/states/playbackvolume と同じ段階・同じ既定値にする。
export const playbackVolumeLevels = ['mute', 'quiet', 'low', 'normal', 'boost'] as const
export type PlaybackVolumeLevel = typeof playbackVolumeLevels[number]
export const defaultPlaybackVolumeLevel: PlaybackVolumeLevel = 'boost'

function isPlaybackVolumeLevel(value: unknown): value is PlaybackVolumeLevel {
  return playbackVolumeLevels.includes(value as PlaybackVolumeLevel)
}

// 再生音量は全端末で共通の値をサーバーが持ち、/ws/playback-volume で接続時と変更時に届く。
// 画面からの変更もサーバーへ送り、ほかの端末へ配られる。切断中の変更はこの画面だけに効き、再接続でサーバーの値に戻る。
export function usePlaybackVolume(backendURL: URL): [PlaybackVolumeLevel, (level: PlaybackVolumeLevel) => void] {
  const [level, setLevel] = useState<PlaybackVolumeLevel>(defaultPlaybackVolumeLevel)
  const socketRef = useRef<WebSocket | null>(null)

  useEffect(() => {
    let disposed = false
    let timer: number | undefined
    let socket: WebSocket
    const connect = () => {
      const url = new URL('/ws/playback-volume', backendURL)
      url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
      socket = new WebSocket(url)
      socketRef.current = socket
      socket.onmessage = event => {
        if (disposed) return
        let message: { type?: string; level?: unknown }
        try { message = JSON.parse(event.data) } catch { return }
        if (message.type === 'playback_volume' && isPlaybackVolumeLevel(message.level)) setLevel(message.level)
      }
      socket.onclose = () => {
        if (disposed) return
        socketRef.current = null
        timer = window.setTimeout(connect, 2000)
      }
      socket.onerror = () => socket.close()
    }
    connect()
    return () => {
      disposed = true
      window.clearTimeout(timer)
      socketRef.current = null
      socket.close()
    }
  }, [backendURL])

  const change = useCallback((next: PlaybackVolumeLevel) => {
    setLevel(next)
    const socket = socketRef.current
    if (socket?.readyState === WebSocket.OPEN) socket.send(JSON.stringify({ type: 'set_playback_volume', level: next }))
  }, [])

  return [level, change]
}
