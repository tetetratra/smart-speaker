import React, { useCallback, useEffect, useState } from 'react'
import { PageLayout } from '../../shared/ui/PageLayout'
import { createMemory, deleteMemory, listMemories, updateMemory, type Memory } from './api'
import './MemoryScreen.css'

function formatMemoryDate(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat('ja-JP', {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(date)
}

// 長期メモリを一覧・追加・編集・削除する画面。開いたときに一覧を取得する。
export function MemoryScreen({ onBack }: { onBack: () => void }) {
  const [memories, setMemories] = useState<Memory[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [newContent, setNewContent] = useState('')
  const [editingID, setEditingID] = useState('')
  const [editingContent, setEditingContent] = useState('')
  const [pendingAction, setPendingAction] = useState('')

  useEffect(() => {
    const controller = new AbortController()
    listMemories(controller.signal)
      .then(setMemories)
      .catch((err) => {
        if (!controller.signal.aborted) setError(err instanceof Error ? err.message : 'メモリ一覧を取得できませんでした')
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })
    return () => controller.abort()
  }, [])

  // 操作を実行し、失敗したらエラーを出す。成功したかを返す。
  const run = useCallback(async (action: string, operation: () => Promise<void>) => {
    setPendingAction(action)
    setError('')
    try {
      await operation()
      return true
    } catch (err) {
      setError(err instanceof Error ? err.message : 'メモリを保存できませんでした')
      return false
    } finally {
      setPendingAction('')
    }
  }, [])
  const cancelEdit = useCallback(() => {
    setEditingID('')
    setEditingContent('')
  }, [])
  const submitNew = useCallback(async () => {
    const content = newContent.trim()
    if (!content) return
    const ok = await run('create', async () => {
      const memory = await createMemory(content)
      setMemories((prev) => [memory, ...prev.filter((item) => item.id !== memory.id)])
    })
    if (ok) setNewContent('')
  }, [newContent, run])
  const submitEdit = useCallback(async (id: string) => {
    const content = editingContent.trim()
    if (!content) return
    const ok = await run(`update:${id}`, async () => {
      const memory = await updateMemory(id, content)
      setMemories((prev) => prev.map((item) => item.id === id ? memory : item))
    })
    if (ok) cancelEdit()
  }, [cancelEdit, editingContent, run])
  const submitDelete = useCallback(async (memory: Memory) => {
    if (!window.confirm('このメモリを削除しますか？')) return
    const ok = await run(`delete:${memory.id}`, async () => {
      await deleteMemory(memory.id)
      setMemories((prev) => prev.filter((item) => item.id !== memory.id))
    })
    if (ok && editingID === memory.id) cancelEdit()
  }, [cancelEdit, editingID, run])

  return (
    <PageLayout onBack={onBack}>
      {error && <div className="error-banner">{error}</div>}
      <div className="panel">
        <div className="memory-list">
          <div className="memory-new">
            <textarea
              className="memory-textarea"
              value={newContent}
              onChange={(event) => setNewContent(event.currentTarget.value)}
              placeholder="追加するメモリ文"
              rows={3}
            />
            <button
              className="memory-btn primary memory-new-submit"
              onClick={() => void submitNew()}
              disabled={!newContent.trim() || pendingAction === 'create'}
            >
              {pendingAction === 'create' ? '追加中' : '追加'}
            </button>
          </div>
          {!loading && !error && memories.length === 0 && (
            <div className="memory-empty">保存済みメモリはありません</div>
          )}
          {memories.map((memory) => (
            <div key={memory.id} className="memory-item">
              <div className="memory-item-main">
                {editingID === memory.id ? (
                  <textarea
                    className="memory-textarea"
                    value={editingContent}
                    onChange={(event) => setEditingContent(event.currentTarget.value)}
                    rows={3}
                  />
                ) : (
                  <div className="memory-item-content">{memory.content}</div>
                )}
                <div className="memory-item-dates">
                  <span>作成: {formatMemoryDate(memory.created_at)}</span>
                  <span>更新: {formatMemoryDate(memory.updated_at)}</span>
                </div>
              </div>
              <div className="memory-item-actions">
                {editingID === memory.id ? (
                  <>
                    <button
                      className="memory-btn primary"
                      onClick={() => void submitEdit(memory.id)}
                      disabled={!editingContent.trim() || pendingAction === `update:${memory.id}`}
                    >
                      {pendingAction === `update:${memory.id}` ? '保存中' : '保存'}
                    </button>
                    <button className="memory-btn" onClick={cancelEdit} disabled={pendingAction === `update:${memory.id}`}>
                      キャンセル
                    </button>
                  </>
                ) : (
                  <>
                    <button
                      className="memory-btn"
                      onClick={() => {
                        setEditingID(memory.id)
                        setEditingContent(memory.content)
                      }}
                      disabled={pendingAction !== ''}
                    >
                      編集
                    </button>
                    <button
                      className="memory-btn danger"
                      onClick={() => void submitDelete(memory)}
                      disabled={pendingAction === `delete:${memory.id}`}
                    >
                      {pendingAction === `delete:${memory.id}` ? '削除中' : '削除'}
                    </button>
                  </>
                )}
              </div>
            </div>
          ))}
        </div>
      </div>
    </PageLayout>
  )
}
