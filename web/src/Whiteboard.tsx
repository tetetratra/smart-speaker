import React, { useEffect, useMemo, useRef } from 'react'
import type { PlaybackSpeech } from './audio'
import { createWhiteboardAutoScroll, whiteboardScrollSpeed, WHITEBOARD_FIXED_SPEED, type SpeechTiming, type WhiteboardAutoScroll } from './whiteboardScroll'
import { whiteboardEvents, type HistoryEntry } from './whiteboardHistory'
export type { HistoryEntry } from './whiteboardHistory'

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
        .live-tool-entry > summary.inbound { background: #fff0df; border-color: #e6bd93; color: #75451f; }
        .live-tool-entry .live-tool-toast-tool { display: flex; flex: 1; min-width: 0; white-space: nowrap; }
        .live-tool-entry-name { flex-shrink: 0; white-space: pre; }
        .live-tool-entry-title { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: pre; }
        .live-tool-entry-status { flex-shrink: 0; font-size: 10px; }
        .live-tool-entry-body { font-size: 12px; padding: 8px; border: 1px solid var(--live-line); border-radius: 8px; margin-top: 4px; overflow-wrap: anywhere; }
        .live-tool-entry-body p { margin: 0 0 8px; color: var(--live-muted); }
        .live-tool-entry-body h3 { margin: 8px 0 4px; font-size: 12px; }
        .live-tool-entry-body pre { margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; font-size: 11px; }
      `}</style>
      <div className="live-board-content" ref={contentRef}>
        {events.map(event => {
          if (event.kind === 'board') {
            return <div key={event.id} className="live-board-event live-board-entry">{event.content}</div>
          }
          const entry = event.entry
          const title = entry.title?.trim()
          // 実行中は緑（call）、結果が返ったら青（result）、失敗なら赤で表示する
          const state = entry.kind === 'inbound' ? 'inbound' : entry.status === 'running' ? 'call' : entry.status === 'failed' ? 'result failed' : 'result'
          return (
            <details key={event.id} className="live-board-event live-tool-entry" onToggle={event => {
              if (event.currentTarget.open) follow.current = false
            }}>
              <summary className={`live-tool-toast ${state}`}>
                <span className="live-tool-toast-tool">
                  <span className="live-tool-entry-name">{entry.name}{title && ': '}</span>
                  {title && <span className="live-tool-entry-title" title={entry.title!}>{title}</span>}
                </span>
                <span className="live-tool-entry-status">▾</span>
              </summary>
              <div className="live-tool-entry-body">
                {entry.kind === 'inbound' ? <>
                  <p>{new Date(event.timestamp).toLocaleString('ja-JP')}<br />出どころ: {entry.source}</p>
                  <h3>モデルに渡した本文</h3>
                  <pre>{entry.text}</pre>
                </> : <>
                  <p>{new Date(event.timestamp).toLocaleString('ja-JP')}{entry.status === 'failed' && '（失敗）'}<br />呼び出しID: {entry.call_id || '（なし）'}</p>
                  <h3>引数</h3>
                  <pre>{prettyJSON(entry.arguments)}</pre>
                  <h3>レスポンス</h3>
                  <pre>{entry.status === 'running' ? '結果を待っています…' : prettyJSON(entry.output)}</pre>
                </>}
              </div>
            </details>
          )
        })}
      </div>
    </div>
  )
}
