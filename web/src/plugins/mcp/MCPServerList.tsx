import React, { useCallback, useEffect, useState } from 'react'
import { apiURL } from '../../shared/backend'
import { listMCPServers, type MCPServer } from './api'
import './MCPServerList.css'

// stateLabel は、状態を画面に出す言葉にする。認可するサーバーは、認可済みかどうかが分かる言葉にする。
export function stateLabel(server: MCPServer): string {
  switch (server.state) {
    case 'ready':
      return server.oauth ? `認可済み・ツール${server.tool_count}個` : `接続済み・ツール${server.tool_count}個`
    case 'unauthorized':
      return '未認可'
    case 'error':
      return '接続できません'
    case 'disabled':
      return '無効'
  }
}

// 設定画面の MCP サーバーの欄の中身。未認可のサーバーには認可のボタンを出し、認可済みのサーバーには認可済みと出す。
// 設定画面を開いている間だけ置かれるので、置かれるたびに（設定画面を開くたびに）一覧を取り直す。
export function MCPServerList() {
  const [servers, setServers] = useState<MCPServer[]>([])
  const [error, setError] = useState('')
  useEffect(() => {
    const controller = new AbortController()
    listMCPServers(controller.signal)
      .then(setServers)
      .catch((err) => {
        if (!controller.signal.aborted) setError(err instanceof Error ? err.message : 'MCPサーバーの一覧を取得できませんでした')
      })
    return () => controller.abort()
  }, [])
  // 認可は Google などの画面へ移るので、画面ごと移動する。終わると設定画面へ戻ってくる。
  const authorize = useCallback((server: MCPServer) => {
    if (!server.auth_url) return
    window.location.href = apiURL(server.auth_url)
  }, [])
  return (
    <div className="mcp-servers">
      {error && <div className="mcp-servers-error">{error}</div>}
      {!error && servers.length === 0 && (
        <div className="mcp-servers-empty">config.yaml に登録された MCP サーバーはありません</div>
      )}
      {servers.map((server) => (
        <div key={server.name} className="mcp-server">
          <div className="mcp-server-main">
            <div className="mcp-server-name">{server.name}</div>
            {/* 接続できないときと未認可のときは、目立つよう赤で出す */}
            <div className={`mcp-server-state${server.state === 'error' || server.state === 'unauthorized' ? ' danger' : ''}`}>
              {stateLabel(server)}
            </div>
            {server.message && <div className="mcp-server-message">{server.message}</div>}
          </div>
          {server.auth_url && (
            <button className="mcp-server-authorize" onClick={() => authorize(server)}>認可する</button>
          )}
        </div>
      ))}
    </div>
  )
}
