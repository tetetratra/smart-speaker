import React, { useLayoutEffect, useRef } from 'react'
import { PageLayout } from '../../shared/ui/PageLayout'
import type { EventLogLine } from './useEventLog'
import './EventLogScreen.css'

function isScrolledToBottom(el: HTMLElement): boolean {
  return el.scrollHeight - el.scrollTop - el.clientHeight <= 4
}

// サーバーから届いたメッセージを 1 行ずつ出すログ画面。
// 開いたときは末尾を表示し、行が増えたら末尾を追う。利用者が上へスクロールしたら追うのをやめ、末尾まで戻したらまた追う。
export function EventLogScreen({ lines, onBack }: { lines: EventLogLine[]; onBack: () => void }) {
  const ref = useRef<HTMLDivElement>(null)
  const follow = useRef(true)
  useLayoutEffect(() => {
    const el = ref.current
    if (el && follow.current) el.scrollTop = el.scrollHeight
  }, [lines])
  return (
    <PageLayout onBack={onBack}>
      <div
        className="panel inset eventlog-panel"
        ref={ref}
        onScroll={() => {
          if (ref.current) follow.current = isScrolledToBottom(ref.current)
        }}
      >
        {lines.map((log) => (
          <div key={log.id} className="eventlog-line">{log.line}</div>
        ))}
      </div>
    </PageLayout>
  )
}
