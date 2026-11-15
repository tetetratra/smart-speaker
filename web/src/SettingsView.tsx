import React, { useCallback, useEffect, useRef, useState } from 'react'
import { WakeModeControl, type WakeModeState } from './WakeModeControl'
import { PlaybackVolumeSlider } from './plugins/playbackvolume/PlaybackVolumeSlider'
import type { PlaybackVolumeLevel } from './plugins/playbackvolume/playbackVolume'

// メイン画面からは外した、ふだん触らない設定と状態表示をまとめた画面。
export function SettingsView(props: {
  visible: boolean
  backendURL: URL
  connectionStatus: string
  speechDetectStatus: string
  sttStatus: string
  wakeMode: WakeModeState
  playbackVolumeLevel: PlaybackVolumeLevel
  onPlaybackVolumeChange: (level: PlaybackVolumeLevel) => void
  goApp: () => void
}) {
  const { visible, backendURL, connectionStatus, speechDetectStatus, sttStatus, wakeMode, playbackVolumeLevel, onPlaybackVolumeChange, goApp } = props
  return (
    <div
      style={{
        display: visible ? 'flex' : 'none',
        flexDirection: 'column',
        gap: 12,
        padding: 12,
        height: '100vh',
        minHeight: 0,
        overflow: 'hidden',
        boxSizing: 'border-box',
      }}
    >
      <div
        style={{
          flex: '1 1 auto',
          minHeight: 0,
          overflow: 'auto',
          border: '2px solid var(--live-line)',
          borderRadius: 10,
          padding: 12,
          background: 'var(--live-panel-soft)',
          boxShadow: 'inset 0 0 0 1px var(--live-inset-line)',
          display: 'flex',
          flexDirection: 'column',
          gap: 16,
        }}
      >
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
          <div style={{ justifySelf: 'start' }}>
            <WakeModeControl state={wakeMode} />
          </div>
        </SettingsSection>
        <SettingsSection title="読み上げ音声">
          <TTSControl visible={visible} backendURL={backendURL} />
        </SettingsSection>
        <SettingsSection title="再生音量">
          <PlaybackVolumeSlider level={playbackVolumeLevel} onChange={onPlaybackVolumeChange} />
        </SettingsSection>
      </div>
      <button
        onClick={goApp}
        style={{
          alignSelf: 'flex-start',
          borderRadius: 8,
          border: '1px solid var(--live-line-strong)',
          background: 'var(--live-panel)',
          color: 'var(--live-text)',
          padding: '10px 14px',
          fontSize: 14,
          fontWeight: 700,
          cursor: 'pointer',
        }}
      >
        app画面
      </button>
    </div>
  )
}

function SettingsSection({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section style={{ display: 'grid', gap: 6, minWidth: 0 }}>
      <h2 style={{ margin: 0, fontSize: 13, fontWeight: 700, color: 'var(--live-muted)' }}>{title}</h2>
      {children}
    </section>
  )
}

type TTSSelection = {
  provider: string
  providers: { id: string; label: string; available: boolean }[]
}

function TTSControl({ visible, backendURL }: { visible: boolean; backendURL: URL }) {
  const [selection, setSelection] = useState<TTSSelection | null>(null)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')
  const requestID = useRef(0)
  const request = useCallback(async (provider?: string) => {
    const id = ++requestID.current
    setPending(true)
    setError('')
    try {
      const response = await fetch(new URL('/api/tts', backendURL), {
        method: provider === undefined ? 'GET' : 'PUT',
        headers: provider === undefined ? undefined : { 'Content-Type': 'application/json' },
        body: provider === undefined ? undefined : JSON.stringify({ provider }),
        cache: 'no-store',
      })
      if (!response.ok) throw new Error(await response.text() || 'TTSの設定を取得・変更できませんでした')
      const next = await response.json() as TTSSelection
      if (id === requestID.current) setSelection(next)
    } catch (err) {
      if (id === requestID.current) setError(err instanceof Error ? err.message : 'TTSを変更できませんでした')
    } finally {
      if (id === requestID.current) setPending(false)
    }
  }, [backendURL])
  useEffect(() => {
    if (!visible) return
    void request()
    return () => { requestID.current++ }
  }, [request, visible])
  return (
    <div style={{ display: 'grid', gap: 4, fontSize: 12, minWidth: 0 }}>
      <label style={{ display: 'flex', gap: 8, alignItems: 'center', minWidth: 0 }}>
        <select
          aria-label="読み上げ音声"
          value={selection?.provider ?? ''}
          disabled={pending || !selection}
          onChange={(event) => void request(event.currentTarget.value)}
          style={{ flex: 1, minWidth: 0, minHeight: 30, background: 'var(--live-panel-soft)', color: 'var(--live-text)', border: '1px solid var(--live-line)', borderRadius: 8, padding: 4 }}
        >
          {!selection && <option value="">{pending ? '読み込み中' : '取得できません'}</option>}
          {selection?.providers.map((provider) => (
            <option key={provider.id} value={provider.id} disabled={!provider.available}>
              {provider.label}{provider.available ? '' : '（未設定）'}
            </option>
          ))}
        </select>
        {error && <button className="live-admin-btn" onClick={() => void request()} disabled={pending}>再取得</button>}
      </label>
      {error && <span role="alert">{error}</span>}
    </div>
  )
}
