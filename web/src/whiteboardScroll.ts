// ホワイトボードに収まらなくなった書き込みを、一気に飛ばさず少しずつ末尾までスクロールする。
//
// 書き込みでスクロールが要るときは、今の読み上げ区間の80%が終わる頃にちょうど末尾へ届く速さにする。
// 吹き出し（bubbleScroll.ts）と違い、書き込みは割り込みでも打ち消されないので、
// 状態として持つのは「今の速さ」だけにし、割り込みが起きても速さを変えずに末尾までスクロールし切る。
// 速さは run() を呼んだとき（新しい書き込みや読み上げ区間の開始）にだけ変わる。
//
// scrollTop は整数pxに丸められてカクつくため、自動で動かす間は中身を包む要素を transform でずらす。
// 途中で速さを変えたり中身が増えたりするので、CSSの transition ではなく requestAnimationFrame で位置を進める。
import { BUBBLE_SCROLL_END_RATIO } from './bubbleScroll.ts'

// 読み上げに合わせられないとき（ツール呼び出しの表示、読み上げがないときの書き込み）の速さ（px/秒）。
export const WHITEBOARD_FIXED_SPEED = 240

// 残り時間がこれより短いと速くなりすぎるため、決まった速さに切り替える。
const MIN_REMAINING_MS = 300

// 読み上げ区間の開始を画面が受け取った時刻と、区間全体の再生時間。
export type SpeechTiming = { startedAt: number; durationMs?: number }

// 末尾までの残り距離を、今の読み上げ区間の80%地点までの残り時間で割った速さ（px/秒）を返す。
export function whiteboardScrollSpeed(distance: number, speech: SpeechTiming | undefined, now: number) {
  if (distance <= 0 || !speech?.durationMs || speech.durationMs <= 0) return WHITEBOARD_FIXED_SPEED
  const remainingMs = speech.startedAt + speech.durationMs * BUBBLE_SCROLL_END_RATIO - now
  if (remainingMs < MIN_REMAINING_MS) return WHITEBOARD_FIXED_SPEED
  return distance / remainingMs * 1000
}

export type WhiteboardAutoScroll = {
  // 末尾までの残り距離（px）。
  distance(): number
  // 指定の速さで末尾へ向けて動かす。動いている最中なら速さだけを変える。
  run(speed: number): void
  isActive(): boolean
  // その時点の位置で止め、scrollTop へ移し替える。
  stop(): void
}

// element はスクロールする枠、content はその中身をすべて包む要素。
export function createWhiteboardAutoScroll(element: HTMLElement, content: HTMLElement): WhiteboardAutoScroll {
  let active = false
  let base = 0
  let position = 0
  let speed = 0
  let frame = 0
  let last = 0

  // transform は scrollHeight に影響しうるため、transform の影響を受けないレイアウト上の高さから末尾を求める。
  const maxScrollTop = () => {
    const style = getComputedStyle(element)
    const padding = (parseFloat(style.paddingTop) || 0) + (parseFloat(style.paddingBottom) || 0)
    return Math.max(0, content.offsetHeight + padding - element.clientHeight)
  }
  const finish = () => {
    if (!active) return
    active = false
    cancelAnimationFrame(frame)
    element.removeEventListener('wheel', finish)
    element.removeEventListener('touchstart', finish)
    element.removeEventListener('pointerdown', finish)
    content.style.transform = ''
    content.style.willChange = ''
    element.scrollTop = position
  }
  const step = (time: number) => {
    // タブが裏にあった間などに一気に進みすぎないよう、1フレームで進める時間に上限を設ける。
    const elapsed = last ? Math.min(time - last, 100) : 0
    last = time
    const target = maxScrollTop()
    position = Math.min(target, position + speed * elapsed / 1000)
    if (position >= target) {
      finish()
      return
    }
    content.style.transform = `translateY(${base - position}px)`
    frame = requestAnimationFrame(step)
  }

  return {
    distance: () => maxScrollTop() - (active ? position : element.scrollTop),
    run(nextSpeed) {
      if (!(nextSpeed > 0)) return
      speed = nextSpeed
      if (active || maxScrollTop() - element.scrollTop <= 0) return
      active = true
      base = element.scrollTop
      position = base
      last = 0
      // 利用者が触れたら、その時点の位置で止めて手での操作に任せる。
      element.addEventListener('wheel', finish, { passive: true })
      element.addEventListener('touchstart', finish, { passive: true })
      element.addEventListener('pointerdown', finish)
      content.style.willChange = 'transform'
      frame = requestAnimationFrame(step)
    },
    isActive: () => active,
    stop: finish,
  }
}
