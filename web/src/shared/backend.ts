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

// requestJSON は JSON を送受信する。応答の本文が空（削除の 204 など）なら undefined を返す。
// 失敗したら「既定の文言 (状態コード): サーバーの応答本文」の Error を投げる。原因の手がかりを残すため、応答本文も付ける。
// 通信そのものに失敗したときは既定の文言だけの Error を投げる。signal で中断したときは中断のエラーをそのまま投げる。
export async function requestJSON<T>(
  path: string,
  failure: string,
  options: { method?: string; body?: unknown; signal?: AbortSignal } = {},
): Promise<T> {
  let response: Response
  try {
    response = await fetch(apiURL(path), {
      method: options.method ?? 'GET',
      headers: options.body === undefined ? undefined : { 'Content-Type': 'application/json' },
      body: options.body === undefined ? undefined : JSON.stringify(options.body),
      signal: options.signal,
      cache: 'no-store',
    })
  } catch (err) {
    if (options.signal?.aborted) throw err
    throw new Error(failure)
  }
  const text = await response.text()
  if (!response.ok) {
    const detail = text.trim()
    throw new Error(`${failure} (${response.status})${detail ? `: ${detail}` : ''}`)
  }
  return (text ? JSON.parse(text) : undefined) as T
}
