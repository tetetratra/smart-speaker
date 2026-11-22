import { requestJSON } from '../../shared/backend'

// Memory は長期メモリの 1 件。
export type Memory = {
  id: string
  content: string
  created_at: string
  updated_at: string
}

function normalizeMemory(memory: Memory): Memory {
  return {
    id: String(memory.id),
    content: String(memory.content),
    created_at: String(memory.created_at),
    updated_at: String(memory.updated_at),
  }
}

export async function listMemories(signal?: AbortSignal): Promise<Memory[]> {
  const data = await requestJSON<{ memories: Memory[] }>('/api/memories', 'メモリ一覧を取得できませんでした', { signal })
  if (!Array.isArray(data?.memories)) throw new Error('メモリ一覧の形式が不正です')
  return data.memories.map(normalizeMemory)
}

export async function createMemory(content: string): Promise<Memory> {
  const data = await requestJSON<{ memory: Memory }>('/api/memories', 'メモリを追加できませんでした', { method: 'POST', body: { content } })
  return normalizeMemory(data.memory)
}

export async function updateMemory(id: string, content: string): Promise<Memory> {
  const data = await requestJSON<{ memory: Memory }>(`/api/memories/${encodeURIComponent(id)}`, 'メモリを更新できませんでした', { method: 'PUT', body: { content } })
  return normalizeMemory(data.memory)
}

export async function deleteMemory(id: string): Promise<void> {
  await requestJSON<void>(`/api/memories/${encodeURIComponent(id)}`, 'メモリを削除できませんでした', { method: 'DELETE' })
}
