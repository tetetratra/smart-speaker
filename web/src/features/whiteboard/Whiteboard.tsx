import React, { useEffect, useMemo, useRef } from 'react'
import type { PlaybackSpeech } from '../conversation/sessionReducer'
import { createWhiteboardAutoScroll, whiteboardScrollSpeed, WHITEBOARD_FIXED_SPEED, type SpeechTiming, type WhiteboardAutoScroll } from './autoScroll'
import { entryTone, whiteboardEvents, type HistoryEntry } from './history'
import './Whiteboard.css'
export type { HistoryEntry } from './history'

function prettyJSON(value: string) {
  if (!value) return '（なし）'
  try { return JSON.stringify(JSON.parse(value), null, 2) } catch { return value }
}

// speech は今再生中の読み上げ区間。書き込みのスクロールの速さを読み上げに合わせるのに使う。
export function Whiteboard({ tools, speech }: { tools: HistoryEntry[]; speech?: PlaybackSpeech }) {
  const boardRef = useRef<HTMLDivElement>(null)
  const follow = useRef(true)
  const contentRef = useRef<HTMLDivElement>(null)
  const scroller = useRef<WhiteboardAutoScroll>()
  const speechTiming = useRef<SpeechTiming>()
  const boardIDs = useRef<Set<string>>()

  useEffect(() => {
    const board = boardRef.current
    const content = contentRef.current
    if (!board || !content) return
    const current = createWhiteboardAutoScroll(board, content)
    scroller.current = current
    return () => {
      current.stop()
      scroller.current = undefined
    }
  }, [])

  // 読み上げ区間が始まったら、スクロール中であればその区間に合わせて速さを計算し直す。
  // 割り込みで読み上げが止まっても速さは変えず、そのまま末尾までスクロールし切る。
  useEffect(() => {
    if (!speech) {
      speechTiming.current = undefined
      return
    }
    const timing = { startedAt: Date.now(), durationMs: speech.durationMs }
    speechTiming.current = timing
    const current = scroller.current
    if (current?.isActive()) current.run(whiteboardScrollSpeed(current.distance(), timing, timing.startedAt))
  }, [speech])

  const events = useMemo(() => whiteboardEvents(tools), [tools])

  useEffect(() => {
    const board = boardRef.current
    const current = scroller.current
    const previous = boardIDs.current
    const currentIDs = new Set(events.filter(event => event.kind === 'board').map(event => event.id))
    boardIDs.current = currentIDs
    if (!board || !current || !follow.current) return
    if (previous === undefined) {
      // 最初の表示では、既にある内容の末尾へそのまま移る。
      board.scrollTop = board.scrollHeight
      return
    }
    if ([...currentIDs].some(id => !previous.has(id))) {
      // 書き込みは今の読み上げに合わせた速さで動かす。
      current.run(whiteboardScrollSpeed(current.distance(), speechTiming.current, Date.now()))
    } else if (!current.isActive()) {
      // ツール呼び出しの表示は高さがほぼ決まっているので、決まった速さで動かす。
      current.run(WHITEBOARD_FIXED_SPEED)
    }
  }, [events])

  return (
    <div className="live-board" ref={boardRef} aria-label="ホワイトボード" onScroll={() => {
      const board = boardRef.current
      if (board) follow.current = board.scrollHeight - board.clientHeight - board.scrollTop < 32
    }}>
      <div className="live-board-content" ref={contentRef}>
        {events.map(event => {
          if (event.kind === 'board') {
            return <div key={event.id} className="live-board-event live-board-entry">{event.content}</div>
          }
          const entry = event.entry
          const title = entry.title?.trim()
          const tone = entryTone(entry)
          return (
            <details key={event.id} className="live-board-event live-tool-entry" onToggle={event => {
              if (event.currentTarget.open) follow.current = false
            }}>
              <summary className={`live-tool-toast ${tone}`}>
                <span className="live-tool-toast-tool">
                  <span className="live-tool-entry-name">{entry.name}{title && ': '}</span>
                  {title && <span className="live-tool-entry-title" title={entry.title!}>{title}</span>}
                </span>
                <span className="live-tool-entry-status">▾</span>
              </summary>
              <div className="live-tool-entry-body">
                {entry.kind === 'inbound' ? <>
                  <p>{new Date(event.timestamp).toLocaleString('ja-JP')}<br />出どころ: {entry.name}{entry.error && <><br />失敗: {entry.error}</>}</p>
                  <h3>モデルに渡した本文</h3>
                  <pre>{entry.text}</pre>
                </> : <>
                  <p>{new Date(event.timestamp).toLocaleString('ja-JP')}{entry.error && '（失敗）'}<br />呼び出しID: {entry.call_id || '（なし）'}</p>
                  <h3>引数</h3>
                  <pre>{prettyJSON(entry.arguments)}</pre>
                  <h3>レスポンス</h3>
                  <pre>{prettyJSON(entry.output)}</pre>
                </>}
              </div>
            </details>
          )
        })}
      </div>
    </div>
  )
}
