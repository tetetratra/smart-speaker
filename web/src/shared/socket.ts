// 切れたら自動でつなぎ直す WebSocket。JSON の解析と送信もここでまとめる。
// 使い手は、音声接続（/ws/chat）・呼びかけ（/ws/wake-mode）・再生音量（/ws/playback-volume）。

export type SocketHandlers = {
  // 接続を試み始めた。最初の接続と、自動でつなぎ直すたびに呼ぶ。
  onConnecting?: () => void
  onOpen?: () => void
  // 受け取ったメッセージ。JSON として読めないものは raw_message、バイナリは binary_message の形で渡す（ログに出すため）。
  onMessage: (message: any) => void
  // 接続が閉じた。close() で閉じたときは呼ばない。
  onClose?: () => void
  // 閉じてから何 ms 後につなぎ直すかを、連続で失敗した回数（1 から数える）から決める。undefined を返すと諦める。
  retryDelay: (attempt: number) => number | undefined
}

export type Socket = {
  // 開いていないときは送らずに捨てる。
  send: (message: unknown) => void
  // 閉じて、つなぎ直すのもやめる。
  close: () => void
}

// createSocket は作るとすぐに接続する。
export function createSocket(url: string, handlers: SocketHandlers): Socket {
  let socket: WebSocket | null = null
  let timer: number | undefined
  let attempt = 0
  let closed = false

  const connect = () => {
    handlers.onConnecting?.()
    const current = new WebSocket(url)
    socket = current
    current.onopen = () => {
      attempt = 0
      handlers.onOpen?.()
    }
    current.onmessage = (event) => {
      if (closed) return
      if (typeof event.data !== 'string') {
        handlers.onMessage({ type: 'binary_message', data: event.data })
        return
      }
      try {
        handlers.onMessage(JSON.parse(event.data))
      } catch (err) {
        console.warn('ws parse error:', err instanceof Error ? err.message : err)
        handlers.onMessage({ type: 'raw_message', data: event.data })
      }
    }
    current.onclose = () => {
      if (closed || socket !== current) return
      socket = null
      handlers.onClose?.()
      attempt += 1
      const delay = handlers.retryDelay(attempt)
      if (delay === undefined) {
        console.warn(`[ws reconnect] ${url}: retry limit reached`)
        return
      }
      timer = window.setTimeout(connect, delay)
    }
  }
  connect()

  return {
    send(message) {
      if (socket?.readyState === WebSocket.OPEN) socket.send(JSON.stringify(message))
    },
    close() {
      if (closed) return
      closed = true
      window.clearTimeout(timer)
      socket?.close()
      socket = null
    },
  }
}
