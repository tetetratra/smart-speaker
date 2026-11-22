import React from 'react'
import { PageLayout } from '../shared/ui/PageLayout'
import { WakeModeToggle } from '../features/wakemode/WakeModeToggle'
import type { WakeModeState } from '../features/wakemode/useWakeMode'
import { TTSSelect } from '../features/tts/TTSSelect'
import type { ConnectionStatus, MicStatus, RecognitionStatus } from '../features/conversation/sessionReducer'
import { PlaybackVolumeSlider } from '../plugins/playbackvolume/PlaybackVolumeSlider'
import type { PlaybackVolumeLevel } from '../plugins/playbackvolume/usePlaybackVolume'
import './SettingsScreen.css'

const connectionLabels: Record<ConnectionStatus, string> = { offline: 'オフライン', connecting: '接続中', online: 'オンライン' }
const micLabels: Record<MicStatus, string> = { idle: '待機中', detecting: '検出中' }
const recognitionLabels: Record<RecognitionStatus, string> = {
  stopped: '停止中', connecting: '接続中', idle: '待機中', recognizing: '認識中', finalizing: '最終結果待ち', done: '完了', error: 'エラー',
}

// メイン画面からは外した、ふだん触らない設定と状態表示をまとめた画面。
// 状態は会話の状態（sessionReducer）の値で受け取り、日本語の表示名にはここで直す。
export function SettingsScreen(props: {
  connection: ConnectionStatus
  mic: MicStatus
  recognition: RecognitionStatus
  wakeMode: WakeModeState
  playbackVolumeLevel: PlaybackVolumeLevel
  onPlaybackVolumeChange: (level: PlaybackVolumeLevel) => void
  onBack: () => void
}) {
  const { connection, mic, recognition, wakeMode, playbackVolumeLevel, onPlaybackVolumeChange, onBack } = props
  return (
    <PageLayout onBack={onBack}>
      <div className="panel inset settings-panel">
        <SettingsSection title="状態">
          <div className="live-status-grid">
            <div className="live-status-card">
              <div className="live-status-label">接続</div>
              <div className="live-status-value">{connectionLabels[connection]}</div>
            </div>
            <div className="live-status-card">
              <div className="live-status-label">マイク</div>
              <div className="live-status-value">{micLabels[mic]}</div>
            </div>
            <div className="live-status-card">
              <div className="live-status-label">認識</div>
              <div className="live-status-value">{recognitionLabels[recognition]}</div>
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
