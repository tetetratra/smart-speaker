import { useCallback, useRef, useState } from 'react'
import { formatServerEventLog } from './format'

const eventLogLimit = 1000

export type EventLogLine = { id: number; line: string }

// useEventLog はサーバーから届いたメッセージの最新 1000 行を持つ。
// ログ画面を開いていない間もたまり続ける必要があるので、App が持つ。
export function useEventLog(): [EventLogLine[], (raw: unknown) => void] {
  const [lines, setLines] = useState<EventLogLine[]>([])
  const nextID = useRef(0)
  const append = useCallback((raw: unknown) => {
    nextID.current += 1
    const line = { id: nextID.current, line: formatServerEventLog(raw) }
    setLines((prev) => {
      const next = [...prev, line]
      return next.length > eventLogLimit ? next.slice(-eventLogLimit) : next
    })
  }, [])
  return [lines, append]
}
