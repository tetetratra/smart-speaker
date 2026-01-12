declare module 'hark' {
  type HarkOptions = {
    threshold?: number
    interval?: number
    play?: boolean
  }

  type HarkInstance = {
    on: (event: 'speaking' | 'stopped_speaking' | 'volume_change', handler: (...args: any[]) => void) => void
    stop: () => void
  }

  export default function hark(stream: MediaStream, options?: HarkOptions): HarkInstance
}
