import React from 'react'
import { PageLayout } from './shared/ui/PageLayout'
import { WakeModeToggle } from './features/wakemode/WakeModeToggle'
import type { WakeModeState } from './features/wakemode/useWakeMode'
import { TTSSelect } from './features/tts/TTSSelect'
import { PlaybackVolumeSlider } from './plugins/playbackvolume/PlaybackVolumeSlider'
import { MCPServerList } from './plugins/mcp/MCPServerList'
import type { PlaybackVolumeLevel } from './plugins/playbackvolume/usePlaybackVolume'
import './SettingsView.css'

// メイン画面からは外した、ふだん触らない設定と状態表示をまとめた画面。
export function SettingsView(props: {
  connectionStatus: string
  speechDetectStatus: string
  sttStatus: string
  wakeMode: WakeModeState
  playbackVolumeLevel: PlaybackVolumeLevel
  onPlaybackVolumeChange: (level: PlaybackVolumeLevel) => void
  onBack: () => void
}) {
  const { connectionStatus, speechDetectStatus, sttStatus, wakeMode, playbackVolumeLevel, onPlaybackVolumeChange, onBack } = props
  return (
    <PageLayout onBack={onBack}>
      <div className="panel inset settings-panel">
        <SettingsSection title="状態">
          <div className="live-status-grid">
            <div className="live-status-card">
              <div className="live-status-label">接続</div>
              <div className="live-status-value">{connectionStatus}</div>
            </div>
            <div className="live-status-card">
              <div className="live-status-label">マイク</div>
              <div className="live-status-value">{speechDetectStatus}</div>
            </div>
            <div className="live-status-card">
              <div className="live-status-label">認識</div>
              <div className="live-status-value">{sttStatus}</div>
            </div>
          </div>
        </SettingsSection>
        <SettingsSection title="呼びかけ">
          <div className="settings-wake-mode">
            <WakeModeToggle state={wakeMode} />
          </div>
        </SettingsSection>
        <SettingsSection title="読み上げ音声">
          <TTSSelect />
        </SettingsSection>
        <SettingsSection title="再生音量">
          <PlaybackVolumeSlider level={playbackVolumeLevel} onChange={onPlaybackVolumeChange} />
        </SettingsSection>
        <SettingsSection title="MCPサーバー">
          <MCPServerList />
        </SettingsSection>
      </div>
    </PageLayout>
  )
}

function SettingsSection({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="settings-section">
      <h2 className="settings-section-title">{title}</h2>
      {children}
    </section>
  )
}
