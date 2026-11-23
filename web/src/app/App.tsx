import React, { useCallback } from 'react'
import { useVoiceSession } from '../features/conversation/useVoiceSession'
import { useWakeMode } from '../features/wakemode/useWakeMode'
import { useEventLog } from '../features/eventlog/useEventLog'
import { EventLogScreen } from '../features/eventlog/EventLogScreen'
import { MemoryScreen } from '../features/memory/MemoryScreen'
import { AutomationScreen } from '../features/automations/AutomationScreen'
import { playbackVolumeGains, usePlaybackVolume } from '../plugins/playbackvolume/usePlaybackVolume'
import { SceneScreen } from '../plugins/switchbot/SceneScreen'
import { useScreen } from './routes'
import { LiveScreen } from './LiveScreen'
import { SettingsScreen } from './SettingsScreen'

// App は画面をまたいで生きる状態（音声接続・呼びかけ・再生音量・ログ）を持ち、機能どうしをつなぐ。
// 再生音量を音声接続の音量へ、受信メッセージをログへつなぐ。
export function App() {
  const [screen, navigate] = useScreen()
  const [eventLog, appendEventLog] = useEventLog()
  const [playbackVolumeLevel, setPlaybackVolumeLevel] = usePlaybackVolume()
  const wakeMode = useWakeMode()
  const voice = useVoiceSession(playbackVolumeGains[playbackVolumeLevel], appendEventLog)
  const goLive = useCallback(() => navigate('live'), [navigate])
  return (
    <>
      {/* メイン画面は、ホワイトボードのスクロール位置を保つために常にマウントしておき、ほかの画面を開いている間は隠す。 */}
      <div hidden={screen !== 'live'}>
        <LiveScreen session={voice.state} connect={voice.connect} disconnect={voice.disconnect} wakeMode={wakeMode} navigate={navigate} />
      </div>
      {screen === 'eventlog' && <EventLogScreen lines={eventLog} onBack={goLive} />}
      {screen === 'memory' && <MemoryScreen onBack={goLive} />}
      {screen === 'jobs' && <AutomationScreen onBack={goLive} />}
      {screen === 'scenes' && <SceneScreen onBack={goLive} />}
      {screen === 'settings' && (
        <SettingsScreen
          connection={voice.state.connection}
          mic={voice.state.mic}
          recognition={voice.state.recognition}
          wakeMode={wakeMode}
          playbackVolumeLevel={playbackVolumeLevel}
          onPlaybackVolumeChange={setPlaybackVolumeLevel}
          onBack={goLive}
        />
      )}
    </>
  )
}
