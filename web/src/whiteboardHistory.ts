type ToolEntry = {
  kind: 'tool'
  id: string
  role: string
  call_id: string
  name: string
  title?: string | null
  arguments: string
  output: string
  error?: string
  started_at: string
}
type InboundEntry = {
  kind: 'inbound'
  id: string
  role: string
  name: string
  title?: string | null
  text: string
  error?: string
  started_at: string
}
export type HistoryEntry = ToolEntry | InboundEntry

// WhiteboardRecord はサーバーが送る会話履歴の記録。ユーザーの発話はチャット欄、それ以外はホワイトボードに出す。
export type WhiteboardRecord = {
  id: string
  role: string
  text: string
  source: string
  generation_id?: number
  title?: string
  external?: boolean
  call_id?: string
  arguments?: string
  output?: string
  error?: string
  created_at: string
}

// mergeRecords は接続後に届いた記録を足す。同じ ID が届いても重複させない。
export function mergeRecords(current: Map<string, WhiteboardRecord>, records: WhiteboardRecord[]): Map<string, WhiteboardRecord> {
  const next = new Map(current)
  for (const record of records) next.set(record.id, record)
  return next
}

// historyEntries は記録から表示用の履歴を組み立てる。
// ツールの記録（tool）は呼び出しと結果を1件で持つので、引数と出力を出す。
// それ以外の記録（後から届いた結果や通知）は、本文を出す。
// 見出しの名前は、ツールかどうかによらず記録の source（中身を作った機能）をそのまま出す。
export function historyEntries(records: Iterable<WhiteboardRecord>): HistoryEntry[] {
  return [...records].map((record): HistoryEntry => record.role === 'tool' ? {
    kind: 'tool', id: record.id, role: record.role, call_id: record.call_id ?? '', name: record.source,
    title: record.title, arguments: record.arguments ?? '', output: record.output ?? '',
    error: record.error || undefined, started_at: record.created_at,
  } : {
    kind: 'inbound', id: record.id, role: record.role, name: record.source,
    title: record.title, text: record.text, error: record.error || undefined, started_at: record.created_at,
  })
}

export type EntryTone = 'tool' | 'background' | 'system' | 'failed'

// entryTone は項目の色を決める。error があれば Role によらず失敗（赤）、
// なければ Role ごとに tool は青、background は緑、system（ほかの Role も）はオレンジにする。
export function entryTone(entry: HistoryEntry): EntryTone {
  if (entry.error) return 'failed'
  if (entry.role === 'tool' || entry.role === 'background') return entry.role
  return 'system'
}
export type BoardEvent =
  | { id: string; kind: 'board'; content: string }
  | { id: string; kind: 'history'; entry: HistoryEntry; timestamp: number }

// 履歴全体から表示を組み立て、ツールの結果は対応する呼び出しの直後に置く。
export function whiteboardEvents(tools: HistoryEntry[]): BoardEvent[] {
  return [...tools].sort((a, b) => Date.parse(a.started_at) - Date.parse(b.started_at)).flatMap(entry => {
    const events: BoardEvent[] = [{ id: `tool-${entry.id}`, kind: 'history', entry, timestamp: Date.parse(entry.started_at) }]
    if (entry.kind !== 'tool' || entry.error) return events
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
