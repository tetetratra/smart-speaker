import { useCallback, useState } from 'react'
import { MCPServerList } from './plugins/mcp/MCPServerList'

// restartPollInterval は、再起動を頼んだあと、サーバーが戻ったかを確かめる間隔。
const restartPollInterval = 1000
// restartTimeout は、サーバーが戻るのを待つ上限。
const restartTimeout = 60_000

type RestartState = 'idle' | 'restarting' | 'done' | 'failed'

// waitForServer は、サーバーが応答するようになるまで待つ。上限を過ぎたら false を返す。
async function waitForServer(backendURL: URL): Promise<boolean> {
  const deadline = Date.now() + restartTimeout
  // 終了の処理が始まる前に応答してしまわないよう、少し待ってから確かめ始める。
  await new Promise((resolve) => setTimeout(resolve, restartPollInterval * 2))
  while (Date.now() < deadline) {
    try {
      const resp = await fetch(new URL('/api/mcp/servers', backendURL).toString(), { cache: 'no-store' })
      if (resp.ok) return true
    } catch {
      // 起動し直している間はつながらないので、待ち続ける。
    }
    await new Promise((resolve) => setTimeout(resolve, restartPollInterval))
  }
  return false
}

// 設定画面。MCP サーバーの認可と、サーバーの再起動ができる。
export function SettingsView(props: { visible: boolean; backendURL: URL; goApp: () => void }) {
  const { visible, backendURL, goApp } = props
  const [restartState, setRestartState] = useState<RestartState>('idle')
  // 再起動のあとに MCP サーバーの一覧を取り直すための番号。
  const [reloadKey, setReloadKey] = useState(0)
  const restart = useCallback(async () => {
    if (!window.confirm('サーバーを再起動します。再起動の間、会話は数秒止まります。よろしいですか？')) return
    setRestartState('restarting')
    try {
      const resp = await fetch(new URL('/api/restart', backendURL).toString(), { method: 'POST' })
      if (!resp.ok) throw new Error(`再起動できませんでした (${resp.status})`)
      const ok = await waitForServer(backendURL)
      setRestartState(ok ? 'done' : 'failed')
    } catch {
      setRestartState('failed')
    }
    setReloadKey((key) => key + 1)
  }, [backendURL])
  const restartMessage = {
    idle: '',
    restarting: '再起動しています…',
    done: '再起動しました。会話がつながっていなければ、app画面で「接続」を押してください。',
    failed: '再起動を確認できませんでした。しばらく待ってから画面を開き直してください。',
  }[restartState]
  return (
    <div
      style={{
        display: visible ? 'flex' : 'none',
        flexDirection: 'column',
        gap: 16,
        padding: 12,
        height: '100vh',
        minHeight: 0,
        overflow: 'auto',
        boxSizing: 'border-box',
      }}
    >
      <MCPServerList backendURL={backendURL} visible={visible} reloadKey={reloadKey} />
      <section style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
        <h2 style={{ margin: 0, fontSize: 16 }}>サーバー</h2>
        <div style={{ fontSize: 13, color: 'var(--live-muted)' }}>
          MCPサーバーを認可したあとや、config.yaml を変えたあとは、再起動すると反映されます。
        </div>
        <button
          onClick={() => void restart()}
          disabled={restartState === 'restarting'}
          style={{
            alignSelf: 'flex-start',
            borderRadius: 8,
            border: '1px solid var(--live-line-strong)',
            background: 'var(--live-panel)',
            color: 'var(--live-text)',
            padding: '10px 14px',
            fontSize: 14,
            fontWeight: 700,
            cursor: restartState === 'restarting' ? 'not-allowed' : 'pointer',
          }}
        >
          再起動
        </button>
        {restartMessage && <div style={{ fontSize: 13, fontWeight: 700 }}>{restartMessage}</div>}
      </section>
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
