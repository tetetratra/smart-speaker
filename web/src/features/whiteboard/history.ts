import type { ConversationRecord } from '../conversation/protocol.ts'

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

// splitRecords は画面に届いた記録を、どこに出すかで分ける。ユーザーの発話はチャット欄、それ以外はホワイトボードに出す。
// 世代のない記録は generation_id が 0 で届くので、世代なし（undefined）として扱う。
export function splitRecords(records: ConversationRecord[]) {
  const utterances: { text: string; generationId?: number }[] = []
  const others: ConversationRecord[] = []
  for (const record of records) {
    if (record.role !== 'user') {
      others.push(record)
      continue
    }
    const generation = record.generation_id
    utterances.push({ text: record.text, generationId: Number.isSafeInteger(generation) && generation! > 0 ? generation : undefined })
  }
  return { utterances, others }
}

// mergeRecords は接続後に届いた記録を足す。同じ ID が届いても重複させない。
export function mergeRecords(current: Map<string, ConversationRecord>, records: ConversationRecord[]): Map<string, ConversationRecord> {
  const next = new Map(current)
  for (const record of records) next.set(record.id, record)
  return next
}

// historyEntries は記録から表示用の履歴を組み立てる。
// ツールの記録（tool）は呼び出しと結果を1件で持つので、引数と出力を出す。
// それ以外の記録（後から届いた結果や通知）は、本文を出す。
// 見出しの名前は、ツールかどうかによらず記録の source（中身を作った機能）をそのまま出す。
export function historyEntries(records: Iterable<ConversationRecord>): HistoryEntry[] {
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
