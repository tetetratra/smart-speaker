import React, { useMemo } from 'react'
import { Whiteboard } from '../features/whiteboard/Whiteboard'
import { historyEntries } from '../features/whiteboard/history'
import { SpeechBubble } from '../features/conversation/SpeechBubble'
import { bubbleOf, type SessionState } from '../features/conversation/sessionReducer'
import type { WakeModeState } from '../features/wakemode/useWakeMode'
import type { Screen } from './routes'
import './LiveScreen.css'

// メイン画面。ホワイトボード・吹き出し・操作ボタン・キャラクターエリアを並べる。
export function LiveScreen(props: {
  session: SessionState
  connect: () => void
  disconnect: () => void
  wakeMode: WakeModeState
  navigate: (screen: Screen) => void
}) {
  const { session, connect, disconnect, wakeMode, navigate } = props
  const tools = useMemo(() => historyEntries(session.records.values()), [session.records])
  const bubble = bubbleOf(session)
  const connected = session.connection === 'online'
  // 接続を試みている間に押しても、確立途中の接続を捨ててやり直さないよう、何もしない。
  const offline = session.connection === 'offline'
  const toggleConnection = () => {
    if (connected) disconnect()
    else if (offline) connect()
  }
  // 呼びかけ状態が分からない間（呼びかけ設定の接続が切れている間）は、呼びかけ待ちとは扱わない。
  const isCharacterDim = !connected || (wakeMode.mode?.required === true && !wakeMode.mode.awake)
  // 暗いとき（話しかけても反応しないとき）にタップすると、接続して呼びかけ中にし、すぐ話せるようにする。
  // 呼びかけは HTTP で音声接続とは独立しているので、接続を待たずに同時に行う。
  const handleCharacterTap = () => {
    if (!isCharacterDim) return
    if (offline) connect()
    void wakeMode.wake()
  }
  return (
    <div className="live-frame">
      <div className="live-main">
        <div className="live-left">
          <Whiteboard tools={tools} speech={session.speech} />
          <SpeechBubble bubble={bubble} speaking={session.speaking} />
        </div>
        <div className="live-right">
          <div className="live-controls-panel">
            {/* 切り替えスイッチと画面遷移ボタンを行で分け、狭い幅でも折り返しで崩れないようにする */}
            <div className="live-controls-toggles">
              <button onClick={toggleConnection} className="live-control-btn" aria-label="接続切替">
                <span className={`live-toggle-switch ${connected ? 'on' : ''}`}></span>
                接続
              </button>
              <button onClick={() => navigate('settings')} className="live-control-btn">設定</button>
            </div>
            <div className="live-controls-nav">
              <button onClick={() => navigate('scenes')} className="live-admin-btn">リモコン</button>
              <button onClick={() => navigate('memory')} className="live-admin-btn">メモリ</button>
              <button onClick={() => navigate('eventlog')} className="live-admin-btn">ログ</button>
              <button onClick={() => navigate('jobs')} className="live-admin-btn">予約</button>
            </div>
          </div>
          <div className="live-character-area">
            <div
              className={`live-volume-control${isCharacterDim ? ' live-character-dim' : ''}`}
              aria-label="キャラクターエリア"
              role="button"
              aria-disabled={!isCharacterDim}
              onClick={handleCharacterTap}
            />
          </div>
        </div>
      </div>
    </div>
  )
}
