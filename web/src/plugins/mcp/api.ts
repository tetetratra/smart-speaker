import { requestJSON } from '../../shared/backend'

// サーバーの状態。サーバー（internal/plugins/mcp/status.go）の値と同じ。
export type MCPServerState = 'ready' | 'unauthorized' | 'restart_required' | 'error' | 'disabled'

export type MCPServer = {
  name: string
  oauth: boolean
  state: MCPServerState
  tool_count: number
  message?: string
  // 認可を始める URL。認可が要るときだけ入っている。
  auth_url?: string
}

export async function listMCPServers(signal?: AbortSignal): Promise<MCPServer[]> {
  const data = await requestJSON<{ servers: MCPServer[] }>('/api/mcp/servers', 'MCPサーバーの一覧を取得できませんでした', { signal })
  return Array.isArray(data?.servers) ? data.servers : []
}
