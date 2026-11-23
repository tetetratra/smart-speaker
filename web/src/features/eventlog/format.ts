// サーバーから届いたメッセージを、ログ画面に出す 1 行の JSON にする。
// 音声などの大きな値は読んでも役に立たず画面も重くなるので「...」に置き換える。

function isBinaryLikeValue(value: unknown): boolean {
  return value instanceof ArrayBuffer || ArrayBuffer.isView(value) || (typeof Blob !== 'undefined' && value instanceof Blob)
}

function isBinaryLikeKey(key: string): boolean {
  return /audio|pcm|sample|prebuffer|bytes|buffer|blob|binary|data/i.test(key)
}

function isBase64Like(value: string): boolean {
  if (value.length < 256 || value.length % 4 !== 0) return false
  return /^[A-Za-z0-9+/]+={0,2}$/.test(value)
}

export function sanitizeEventValue(value: unknown, key = ''): unknown {
  if (isBinaryLikeValue(value)) return '...'
  if (typeof value === 'string') {
    if (isBinaryLikeKey(key) || isBase64Like(value)) return '...'
    return value
  }
  if (Array.isArray(value)) {
    if (isBinaryLikeKey(key) && value.length > 0) return '...'
    return value.map((item) => sanitizeEventValue(item))
  }
  if (value && typeof value === 'object') {
    const source = value as Record<string, unknown>
    // type を先頭に出し、どのメッセージかを行の頭で読めるようにする。
    const result: Record<string, unknown> = {}
    if (Object.prototype.hasOwnProperty.call(source, 'type')) {
      result.type = sanitizeEventValue(source.type, 'type')
    }
    for (const [entryKey, entryValue] of Object.entries(source)) {
      if (entryKey === 'type') continue
      result[entryKey] = sanitizeEventValue(entryValue, entryKey)
    }
    return result
  }
  return value
}

export function formatServerEventLog(raw: unknown): string {
  const sanitized = sanitizeEventValue(raw)
  try {
    return JSON.stringify(sanitized)
  } catch {
    return JSON.stringify({ type: 'unserializable', value: '...' })
  }
}
