import React, { useEffect, useMemo, useRef } from 'react'
import type { PlaybackSpeech } from './audio'
import { createWhiteboardAutoScroll, whiteboardScrollSpeed, WHITEBOARD_FIXED_SPEED, type SpeechTiming, type WhiteboardAutoScroll } from './whiteboardScroll'

type BoardEntry = { id: number; content: string; timestamp: number }
export type ToolEntry = {
  id: string
  call_id: string
  name: string
  arguments: string
  output: string
  status: 'running' | 'succeeded' | 'failed'
  started_at: string
  finished_at?: string
}
type BoardEvent =
  | { id: string; timestamp: number; kind: 'board'; entry: BoardEntry }
  | { id: string; timestamp: number; kind: 'tool'; entry: ToolEntry }

function prettyJSON(value: string) {
  if (!value) return '（なし）'
  try { return JSON.stringify(JSON.parse(value), null, 2) } catch { return value }
}

// speech は今再生中の読み上げ区間。書き込みのスクロールの速さを読み上げに合わせるのに使う。
export function Whiteboard({ entries, tools, speech }: { entries: BoardEntry[]; tools: ToolEntry[]; speech?: PlaybackSpeech }) {
  const boardRef = useRef<HTMLDivElement>(null)
  const follow = useRef(true)
  const contentRef = useRef<HTMLDivElement>(null)
  const scroller = useRef<WhiteboardAutoScroll>()
  const speechTiming = useRef<SpeechTiming>()
  const boardCount = useRef<number>()

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

  const events = useMemo(() => {
    const result: BoardEvent[] = entries.map(entry => ({ id: `board-${entry.id}`, timestamp: entry.timestamp, kind: 'board', entry }))
    // 呼び出しと結果はひとつの表示にまとめ、呼び出し時刻の位置に置く
    for (const entry of tools) {
      result.push({ id: `tool-${entry.id}`, timestamp: Date.parse(entry.started_at), kind: 'tool', entry })
    }
    return result.sort((a, b) => a.timestamp - b.timestamp)
  }, [entries, tools])

  useEffect(() => {
    const board = boardRef.current
    const current = scroller.current
    const previous = boardCount.current
    boardCount.current = entries.length
    if (!board || !current || !follow.current) return
    if (previous === undefined) {
      // 最初の表示では、既にある内容の末尾へそのまま移る。
      board.scrollTop = board.scrollHeight
      return
    }
    if (entries.length > previous) {
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
      <style>{`
        .live-board-content { display: flow-root; }
        .live-board-event + .live-board-event { margin-top: 6px; }
        .live-board-event.live-board-entry + .live-board-entry { border-top: 1px solid var(--live-line); padding-top: 10px; }
        .live-tool-entry { min-width: 0; }
        .live-tool-entry > summary.live-tool-toast { cursor: pointer; list-style: none; }
        .live-tool-entry > summary::-webkit-details-marker { display: none; }
        .live-tool-entry > summary::marker { content: ''; }
        .live-tool-entry > summary:focus-visible { outline: 2px solid var(--live-toggle-on); outline-offset: 2px; }
        .live-tool-entry > summary.failed { background: var(--ui-danger-bg); border-color: var(--ui-danger-line); color: var(--ui-danger-text); }
        .live-tool-entry .live-tool-toast-tool { overflow-wrap: anywhere; white-space: normal; }
        .live-tool-entry-status { flex-shrink: 0; font-size: 10px; }
        .live-tool-entry-body { font-size: 12px; padding: 8px; border: 1px solid var(--live-line); border-radius: 8px; margin-top: 4px; overflow-wrap: anywhere; }
        .live-tool-entry-body p { margin: 0 0 8px; color: var(--live-muted); }
        .live-tool-entry-body h3 { margin: 8px 0 4px; font-size: 12px; }
        .live-tool-entry-body pre { margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; font-size: 11px; }
      `}</style>
      <div className="live-board-content" ref={contentRef}>
        {events.map(event => {
          if (event.kind === 'board') {
            return <div key={event.id} className="live-board-event live-board-entry">{event.entry.content}</div>
          }
          const entry = event.entry
          // 実行中は緑（call）、結果が返ったら青（result）、失敗なら赤で表示する
          const state = entry.status === 'running' ? 'call' : entry.status === 'failed' ? 'result failed' : 'result'
          return (
            <details key={event.id} className="live-board-event live-tool-entry" onToggle={event => {
              if (event.currentTarget.open) follow.current = false
            }}>
              <summary className={`live-tool-toast ${state}`}>
                <span className="live-tool-toast-tool">{entry.name}</span>
                <span className="live-tool-entry-status">▾</span>
              </summary>
              <div className="live-tool-entry-body">
                <p>{new Date(event.timestamp).toLocaleString('ja-JP')}{entry.status === 'failed' && '（失敗）'}<br />呼び出しID: {entry.call_id || '（なし）'}</p>
                <h3>引数</h3>
                <pre>{prettyJSON(entry.arguments)}</pre>
                <h3>レスポンス</h3>
                <pre>{entry.status === 'running' ? '結果を待っています…' : prettyJSON(entry.output)}</pre>
              </div>
            </details>
          )
        })}
      </div>
    </div>
  )
}
