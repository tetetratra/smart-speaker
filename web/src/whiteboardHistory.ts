type ToolEntry = {
  kind: 'tool'
  id: string
  call_id: string
  name: string
  title?: string | null
  arguments: string
  output: string
  status: 'succeeded' | 'failed'
  started_at: string
}
type InboundEntry = {
  kind: 'inbound'
  id: string
  source: string
  name: string
  title?: string | null
  text: string
  started_at: string
}
export type HistoryEntry = ToolEntry | InboundEntry

// WhiteboardRecord はサーバーが送る会話履歴の記録（ホワイトボードに出す、会話でないもの）。
export type WhiteboardRecord = {
  id: string
  role: string
  text: string
  source: string
  title?: string
  external?: boolean
  tool_name?: string
  call_id?: string
  arguments?: string
  output?: string
  created_at: string
}

// mergeRecords は接続後に届いた記録を足す。同じ ID が届いても重複させない。
export function mergeRecords(current: Map<string, WhiteboardRecord>, records: WhiteboardRecord[]): Map<string, WhiteboardRecord> {
  const next = new Map(current)
  for (const record of records) next.set(record.id, record)
  return next
}

// historyEntries は記録から表示用の履歴を組み立てる。
// ツールの記録は呼び出しと結果を1件で持つので、出力に error があれば失敗、なければ成功とする。
// それ以外の記録（後から届いた結果や通知）は、差し込みとして出す。
export function historyEntries(records: Iterable<WhiteboardRecord>): HistoryEntry[] {
  return [...records].map((record): HistoryEntry => record.role === 'tool_call' ? {
    kind: 'tool', id: record.id, call_id: record.call_id ?? '', name: record.tool_name || record.source,
    title: record.title, arguments: record.arguments ?? '', output: record.output ?? '',
    status: failed(record.output) ? 'failed' : 'succeeded', started_at: record.created_at,
  } : {
    kind: 'inbound', id: record.id, source: record.source, name: record.tool_name || record.source,
    title: record.title, text: record.text, started_at: record.created_at,
  })
}

function failed(output?: string) {
  try {
    const value = JSON.parse(output ?? '')
    if (value === null) return false
    return typeof value !== 'object' || Array.isArray(value) || value.error != null
  } catch {
    return true
  }
}
export type BoardEvent =
  | { id: string; kind: 'board'; content: string }
  | { id: string; kind: 'history'; entry: HistoryEntry; timestamp: number }

// 履歴全体から表示を組み立て、ツールの結果は対応する呼び出しの直後に置く。
export function whiteboardEvents(tools: HistoryEntry[]): BoardEvent[] {
  return [...tools].sort((a, b) => Date.parse(a.started_at) - Date.parse(b.started_at)).flatMap(entry => {
    const events: BoardEvent[] = [{ id: `tool-${entry.id}`, kind: 'history', entry, timestamp: Date.parse(entry.started_at) }]
    if (entry.kind !== 'tool' || entry.status !== 'succeeded') return events
    switch (entry.name) {
      case 'set_whiteboard': {
        try {
          const output = JSON.parse(entry.output)
          const content = typeof output?.content === 'string' ? output.content.trim() : ''
          if (content) events.push({ id: `board-${entry.id}`, kind: 'board', content })
        } catch {
          // 結果を解釈できなくても呼び出し履歴は表示する。
        }
        break
      }
    }
    return events
  })
}
