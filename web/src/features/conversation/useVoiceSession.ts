import { useCallback, useEffect, useReducer, useRef } from 'react'
import { createVoiceSession, type VoiceSession } from './voiceSession'
import { initialSessionState, sessionReducer, type SessionState } from './sessionReducer'

// useVoiceSession は音声接続（createVoiceSession）を持ち、その出来事から画面に出す会話の状態を作る。
// volume は再生音量の倍率。onRawMessage には受け取ったメッセージを加工前のまま渡す。
export function useVoiceSession(volume: number, onRawMessage: (message: unknown) => void): {
  state: SessionState
  connect: () => void
  disconnect: () => void
} {
  const [state, dispatch] = useReducer(sessionReducer, undefined, initialSessionState)
  const sessionRef = useRef<VoiceSession | null>(null)
  // 最新のものを ref 経由で呼び、セッションを作り直さない。
  const onRawMessageRef = useRef(onRawMessage)
  onRawMessageRef.current = onRawMessage
  const volumeRef = useRef(volume)
  volumeRef.current = volume

  useEffect(() => {
    const session = createVoiceSession({
      initialVolume: volumeRef.current,
      onEvent: dispatch,
      onRawMessage: (message) => onRawMessageRef.current(message),
    })
    sessionRef.current = session
    return () => {
      session.disconnect()
      sessionRef.current = null
    }
  }, [])

  useEffect(() => {
    sessionRef.current?.setVolume(volume)
  }, [volume])

  const connect = useCallback(() => sessionRef.current?.connect(), [])
  const disconnect = useCallback(() => sessionRef.current?.disconnect(), [])
  return { state, connect, disconnect }
}
