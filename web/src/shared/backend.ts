// バックエンド（Go のサーバー）の URL を作る。
// 画面はふだんサーバーから配られるので同じオリジンを使い、開発時（vite の 5173 番）だけサーバーの 8081 番に向ける。
function backendOrigin(): URL {
  const origin = new URL(window.location.origin)
  if (origin.port === '5173') origin.port = '8081'
  return origin
}

// apiURL は HTTP の API の URL を返す。
export function apiURL(path: string): string {
  return new URL(path, backendOrigin()).toString()
}

// wsURL は WebSocket の URL を返す。https で開いているときは wss にする。
export function wsURL(path: string): string {
  const url = new URL(path, backendOrigin())
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  return url.toString()
}
