// 吹き出しに収まらない読み上げ区間を、再生の進み具合に合わせて上から下へスクロールする。
// 再生開始時は先頭、再生終了時にちょうど末尾へ到達するよう、経過時間に比例させる。
export function bubbleScrollTop(elapsedMs: number, durationMs: number, maxScrollTop: number) {
  if (!(maxScrollTop > 0)) return 0
  if (!(durationMs > 0)) return 0
  const progress = Math.min(1, Math.max(0, elapsedMs / durationMs))
  return maxScrollTop * progress
}

// 要素を再生時間に合わせて自動スクロールし、停止用の関数を返す。
// 利用者が自分でスクロールしようとしたら、その操作を優先して自動スクロールをやめる。
export function startBubbleAutoScroll(element: HTMLElement, durationMs: number | undefined) {
  element.scrollTop = 0
  const maxScrollTop = element.scrollHeight - element.clientHeight
  if (!durationMs || durationMs <= 0 || maxScrollTop <= 0) return () => {}
  const startedAt = performance.now()
  let frame = 0
  const stop = () => {
    cancelAnimationFrame(frame)
    element.removeEventListener('wheel', stop)
    element.removeEventListener('touchstart', stop)
    element.removeEventListener('pointerdown', stop)
  }
  const step = (now: number) => {
    // 再生中に幅が変わっても末尾で止まるよう、毎回スクロール量を測り直す。
    const max = element.scrollHeight - element.clientHeight
    element.scrollTop = bubbleScrollTop(now - startedAt, durationMs, max)
    if (now - startedAt < durationMs) frame = requestAnimationFrame(step)
    else stop()
  }
  element.addEventListener('wheel', stop, { passive: true })
  element.addEventListener('touchstart', stop, { passive: true })
  element.addEventListener('pointerdown', stop)
  frame = requestAnimationFrame(step)
  return stop
}
