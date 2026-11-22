import { useCallback, useEffect, useRef, useState } from 'react'
import { wsURL } from '../../shared/backend'
import { createSocket, type Socket } from '../../shared/socket'

// サーバーの internal/plugins/playbackvolume と同じ段階・同じ既定値にする。
export const playbackVolumeLevels = ['mute', 'quiet', 'low', 'normal', 'boost'] as const
export type PlaybackVolumeLevel = typeof playbackVolumeLevels[number]
export const defaultPlaybackVolumeLevel: PlaybackVolumeLevel = 'low'

// 段階ごとに音声へ掛ける倍率。
export const playbackVolumeGains: Record<PlaybackVolumeLevel, number> = {
  mute: 0,
  quiet: 0.3,
  low: 0.6,
  normal: 1.0,
  boost: 1.5,
}

function isPlaybackVolumeLevel(value: unknown): value is PlaybackVolumeLevel {
  return playbackVolumeLevels.includes(value as PlaybackVolumeLevel)
}

// 再生音量は全端末で共通の値をサーバーが持ち、/ws/playback-volume で接続時と変更時に届く。
// 画面からの変更もサーバーへ送り、ほかの端末へ配られる。切断中の変更はこの画面だけに効き、再接続でサーバーの値に戻る。
export function usePlaybackVolume(): [PlaybackVolumeLevel, (level: PlaybackVolumeLevel) => void] {
  const [level, setLevel] = useState<PlaybackVolumeLevel>(defaultPlaybackVolumeLevel)
  const socketRef = useRef<Socket | null>(null)

  useEffect(() => {
    const socket = createSocket(wsURL('/ws/playback-volume'), {
      onMessage: (message: { type?: string; level?: unknown }) => {
        if (message.type === 'playback_volume' && isPlaybackVolumeLevel(message.level)) setLevel(message.level)
      },
      retryDelay: () => 2000,
    })
    socketRef.current = socket
    return () => {
      socketRef.current = null
      socket.close()
    }
  }, [])

  const change = useCallback((next: PlaybackVolumeLevel) => {
    setLevel(next)
    socketRef.current?.send({ type: 'set_playback_volume', level: next })
  }, [])

  return [level, change]
}
