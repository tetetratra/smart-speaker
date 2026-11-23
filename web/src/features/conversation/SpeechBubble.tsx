import React, { useEffect, useRef } from 'react'
import { startBubbleAutoScroll } from './bubbleScroll'
import type { Bubble } from './sessionReducer'
import './SpeechBubble.css'

// 今の世代のユーザーの発話と読み上げ中の文を出す吹き出し。読み上げ中は枠を光らせる。
// 読み上げ区間が変わるたびに、再生時間に合わせて自動でスクロールする。
export function SpeechBubble({ bubble, speaking }: { bubble: Bubble; speaking: boolean }) {
  const contentRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const element = contentRef.current
    if (!element || !bubble.speech) return
    return startBubbleAutoScroll(element, bubble.speech.durationMs)
  }, [bubble.speech])
  return (
    <div className="live-bubble-slot">
      {bubble.visible && (
        <div className={`live-bubble${speaking ? ' live-bubble-ai-speaking' : ''}`}>
          <div className="live-bubble-content" ref={contentRef}>
            {bubble.user && <div className="live-bubble-user">{bubble.user}</div>}
            {bubble.assistant && <div className="live-bubble-agent">{bubble.assistant}</div>}
          </div>
        </div>
      )}
    </div>
  )
}
