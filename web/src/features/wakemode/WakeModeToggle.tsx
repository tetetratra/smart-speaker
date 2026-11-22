import React from 'react'
import type { WakeModeState } from './useWakeMode'
import './WakeModeToggle.css'

// 呼びかけを必須にするかを切り替えるトグル。切り替えは WebSocket で送り、結果が届くまで押せなくする。
export function WakeModeToggle({ state }: { state: WakeModeState }) {
  const { mode, error, pending, save } = state
  return (
    <div className="wake-mode-toggle">
      <button
        type="button"
        role="switch"
        aria-label="ウェイクアップワード必須"
        aria-checked={mode?.required ?? false}
        aria-busy={pending}
        className="live-control-btn"
        disabled={!mode || pending}
        onClick={() => { if (mode) save(!mode.required) }}
      >
        <span className={`live-toggle-switch ${mode?.required ? 'on' : ''}`} aria-hidden="true" />
        呼びかけ
      </button>
      {error && <span role="alert">{error}</span>}
    </div>
  )
}
