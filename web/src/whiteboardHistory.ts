type ToolEntry = {
  kind: 'tool'
  id: string
  call_id: string
  name: string
  title?: string | null
  arguments: string
  output: string
  status: 'running' | 'succeeded' | 'failed'
  started_at: string
  finished_at?: string
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

// mergeRecords は届いた記録を ID で上書きする。開いた直後の一覧と差分の前後が入れ替わっても崩れない。
export function mergeRecords(current: Map<string, WhiteboardRecord>, records: WhiteboardRecord[]): Map<string, WhiteboardRecord> {
  const next = new Map(current)
  for (const record of records) next.set(record.id, record)
  return next
}

// historyEntries は記録から表示用の履歴を組み立てる。
// ツールの呼び出しに同じ call_id の結果が付けば完了（出力に error があれば失敗）、まだなら実行中とする。
// 呼び出しと対にならない結果や通知は、差し込みとして出す。
export function historyEntries(records: Iterable<WhiteboardRecord>): HistoryEntry[] {
  const list = [...records]
  const results = new Map<string, WhiteboardRecord>()
  for (const record of list) {
    if (record.role === 'tool_result' && record.call_id) results.set(record.call_id, record)
  }
  const calls = new Set(list.filter(record => record.role === 'tool_call' && record.call_id).map(record => record.call_id))
  const entries: HistoryEntry[] = []
  for (const record of list) {
    if (record.role === 'tool_call') {
      const result = record.call_id ? results.get(record.call_id) : undefined
      entries.push({
        kind: 'tool', id: record.id, call_id: record.call_id ?? '', name: record.tool_name || record.source,
        title: record.title, arguments: record.arguments ?? '', output: result?.output ?? '',
        status: result ? (failed(result.output) ? 'failed' : 'succeeded') : 'running',
        started_at: record.created_at, finished_at: result?.created_at,
      })
    } else if (record.role !== 'tool_result' || !record.call_id || !calls.has(record.call_id)) {
      entries.push({
        kind: 'inbound', id: record.id, source: record.source, name: record.tool_name || record.source,
        title: record.title, text: record.text, started_at: record.created_at,
      })
    }
  }
  return entries
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
