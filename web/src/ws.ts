export function createWS(url: string) {
  let socket: WebSocket | null = null

  return {
    connect: (onMessage: (msg: any) => void) => {
      return new Promise<void>((resolve, reject) => {
        socket = new WebSocket(url)
        socket.onopen = () => {
          resolve()
        }
        socket.onerror = (ev) => {
          reject(ev)
        }
        socket.onmessage = (ev) => {
          try {
            const msg = JSON.parse(ev.data)
            onMessage(msg)
          } catch (e) {
            console.warn('ws parse error:', (e as any)?.message)
          }
        }
      })
    },
    send: (msg: any) => {
      if (!socket || socket.readyState !== WebSocket.OPEN) return
      socket.send(JSON.stringify(msg))
    },
    close: () => {
      if (socket) {
        socket.close()
        socket = null
      }
    },
  }
}
