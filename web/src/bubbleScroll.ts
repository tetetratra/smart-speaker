// 吹き出しに収まらない読み上げ区間を、再生の進み具合に合わせて上から下へ動かす。
// 再生開始時は先頭から一定の速さで動かし、再生時間の80%の時点で末尾に届くようにする。
// 最後まで読み上げに合わせると、末尾の行が見えるのが読み終わりと同時になり読みにくいため、少し先に届かせる。
//
// scrollTop は整数pxに丸められ、ゆっくり動かすと1pxずつ段になってカクつくため、
// 自動で動かす間は中身を transform でずらし、CSSの transition に任せる。
// 動き終わったときや利用者が触れたときに、その時点の位置を scrollTop へ移し替えて、
// 以降は通常のスクロールとして手で動かせるようにする。
export const BUBBLE_SCROLL_END_RATIO = 0.8

// getComputedStyle の transform（matrix / matrix3d）から縦方向の移動量を取り出す。
export function translateYFromTransform(transform: string) {
  const match = /^matrix(3d)?\((.+)\)$/.exec(transform.trim())
  if (!match) return 0
  const values = match[2].split(',').map(Number)
  const y = match[1] ? values[13] : values[5]
  return Number.isFinite(y) ? y : 0
}

function setTransform(children: HTMLElement[], transform: string, transition: string) {
  for (const child of children) {
    child.style.transition = transition
    child.style.transform = transform
    child.style.willChange = transform ? 'transform' : ''
  }
}

// 要素の中身を再生時間に合わせて動かし、停止用の関数を返す。
export function startBubbleAutoScroll(element: HTMLElement, durationMs: number | undefined) {
  const children = Array.from(element.children) as HTMLElement[]
  setTransform(children, '', '')
  element.scrollTop = 0
  const maxScrollTop = element.scrollHeight - element.clientHeight
  if (!durationMs || durationMs <= 0 || maxScrollTop <= 0 || children.length === 0) return () => {}

  let active = true
  const finish = (offset: number) => {
    if (!active) return
    active = false
    element.removeEventListener('wheel', interrupt)
    element.removeEventListener('touchstart', interrupt)
    element.removeEventListener('pointerdown', interrupt)
    children[0].removeEventListener('transitionend', complete)
    setTransform(children, '', '')
    element.scrollTop = offset
  }
  // 利用者が触れたら、その時点の見た目の位置で止めて手での操作に任せる。
  const interrupt = () => finish(-translateYFromTransform(getComputedStyle(children[0]).transform))
  const complete = (event: TransitionEvent) => {
    if (event.target === children[0] && event.propertyName === 'transform') finish(maxScrollTop)
  }

  element.addEventListener('wheel', interrupt, { passive: true })
  element.addEventListener('touchstart', interrupt, { passive: true })
  element.addEventListener('pointerdown', interrupt)
  children[0].addEventListener('transitionend', complete)
  setTransform(children, 'translateY(0px)', 'none')
  // 開始位置を描画に確定させてから transition を設定しないと、アニメーションせずに末尾へ飛ぶ。
  void element.offsetHeight
  setTransform(children, `translateY(${-maxScrollTop}px)`, `transform ${durationMs * BUBBLE_SCROLL_END_RATIO}ms linear`)
  // 区間が変わったときなどに呼ばれる。動きかけの状態は次の区間の開始時に先頭へ戻す。
  return () => {
    if (active) interrupt()
  }
}
