type ToolEntry = {
  kind: 'tool'
  id: string
  call_id: string
  name: string
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
  text: string
  started_at: string
}
export type HistoryEntry = ToolEntry | InboundEntry
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
