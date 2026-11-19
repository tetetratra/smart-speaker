import { useCallback, useEffect, useState } from 'react'

// サーバーの状態。サーバー（internal/plugins/mcp/status.go）の値と同じ。
type MCPServerState = 'ready' | 'unauthorized' | 'restart_required' | 'error' | 'disabled'

export type MCPServer = {
  name: string
  oauth: boolean
  state: MCPServerState
  tool_count: number
  message?: string
  // 認可を始める URL。認可が要るときだけ入っている。
  auth_url?: string
}

type MCPServerListResponse = { servers: MCPServer[] }

// stateLabel は、状態を画面に出す言葉にする。認可するサーバーは、認可済みかどうかが分かる言葉にする。
export function stateLabel(server: MCPServer): string {
  switch (server.state) {
    case 'ready':
      return server.oauth ? `認可済み・ツール${server.tool_count}個` : `接続済み・ツール${server.tool_count}個`
    case 'unauthorized':
      return '未認可'
    case 'restart_required':
      return '認可済み・再起動すると使えます'
    case 'error':
      return '接続できません'
    case 'disabled':
      return '無効'
  }
}

// MCP サーバーの一覧を取る。reloadKey が変わるたびに取り直す。
export function useMCPServers(backendURL: URL, visible: boolean, reloadKey: number) {
  const [servers, setServers] = useState<MCPServer[]>([])
  const [error, setError] = useState('')
  useEffect(() => {
    if (!visible) return
    const controller = new AbortController()
    setError('')
    fetch(new URL('/api/mcp/servers', backendURL).toString(), { signal: controller.signal })
      .then(async (resp) => {
        if (!resp.ok) throw new Error(`MCPサーバーの一覧を取得できませんでした (${resp.status})`)
        return await resp.json() as MCPServerListResponse
      })
      .then((data) => setServers(Array.isArray(data.servers) ? data.servers : []))
      .catch((err) => {
        if (controller.signal.aborted) return
        setError(err instanceof Error ? err.message : 'MCPサーバーの一覧を取得できませんでした')
      })
    return () => controller.abort()
  }, [backendURL, visible, reloadKey])
  return { servers, error }
}

// 設定画面の MCP サーバーの欄。未認可のサーバーには認可のボタンを出し、認可済みのサーバーには認可済みと出す。
export function MCPServerList(props: { backendURL: URL; visible: boolean; reloadKey: number }) {
  const { backendURL, visible, reloadKey } = props
  const { servers, error } = useMCPServers(backendURL, visible, reloadKey)
  // 認可は Google などの画面へ移るので、画面ごと移動する。終わると設定画面へ戻ってくる。
  const authorize = useCallback((server: MCPServer) => {
    if (!server.auth_url) return
    window.location.href = new URL(server.auth_url, backendURL).toString()
  }, [backendURL])
  return (
    <section style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
      <h2 style={{ margin: 0, fontSize: 16 }}>MCPサーバー</h2>
      {error && <div style={{ color: 'var(--ui-danger-text)', fontSize: 13, fontWeight: 700 }}>{error}</div>}
      {!error && servers.length === 0 && (
        <div style={{ color: 'var(--live-muted)', fontSize: 14 }}>config.yaml に登録された MCP サーバーはありません</div>
      )}
      {servers.map((server) => (
        <div
          key={server.name}
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 12,
            border: '1px solid var(--live-line)',
            borderRadius: 8,
            background: 'var(--live-panel)',
            padding: '10px 12px',
          }}
        >
          <div style={{ flex: 1, minWidth: 0 }}>
            <div style={{ fontSize: 15, fontWeight: 800, overflowWrap: 'anywhere' }}>{server.name}</div>
            <div
              style={{
                fontSize: 13,
                fontWeight: 700,
                color: server.state === 'error' || server.state === 'unauthorized' ? 'var(--ui-danger-text)' : 'var(--live-muted)',
              }}
            >
              {stateLabel(server)}
            </div>
            {server.message && (
              <div style={{ fontSize: 12, color: 'var(--live-muted)', overflowWrap: 'anywhere' }}>{server.message}</div>
            )}
          </div>
          {server.auth_url && (
            <button
              onClick={() => authorize(server)}
              style={{
                flexShrink: 0,
                borderRadius: 8,
                border: 'none',
                background: 'var(--ui-accent)',
                color: 'var(--ui-accent-text)',
                padding: '8px 14px',
                fontSize: 14,
                fontWeight: 700,
                cursor: 'pointer',
              }}
            >
              認可する
            </button>
          )}
        </div>
      ))}
    </section>
  )
}
