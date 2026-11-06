import React, { useEffect, useRef, useState } from 'react'

type WakeMode = { required: boolean; word: string; min_confidence: number; min_similarity: number; last?: {confidence: number; similarity: number; accepted: boolean; failed: boolean}; revision: number }
export function WakeModeControl({ backendURL }: { backendURL: URL }) {
  const [mode, setMode] = useState<WakeMode>()
  const [word, setWord] = useState('')
  const [confidence, setConfidence] = useState(0.55)
  const [similarity, setSimilarity] = useState(0.8)
  const [error, setError] = useState('')
  const [pending, setPending] = useState(false)
  const dirty = useRef(false)
  const writing = useRef(false)
  const sequence = useRef(0)
  useEffect(() => {
    const controller = new AbortController()
    let loading = false
    const refresh = async () => {
      if (loading || writing.current) return
      loading = true
      const id = ++sequence.current
      try {
        const response = await fetch(new URL('/api/wake-mode', backendURL), { signal: controller.signal, cache: 'no-store' })
        if (!response.ok) throw new Error('待機モードを取得できませんでした')
        const next = await response.json() as WakeMode
        if (id !== sequence.current || controller.signal.aborted) return
        setMode(next); if (!dirty.current) {setWord(next.word); setConfidence(next.min_confidence); setSimilarity(next.min_similarity)}; setError('')
      } catch (err) { if (!controller.signal.aborted && id === sequence.current) setError(err instanceof Error ? err.message : '取得できませんでした') }
      finally { loading = false }
    }
    void refresh()
    const timer = window.setInterval(() => void refresh(), 2000)
    return () => { controller.abort(); window.clearInterval(timer); sequence.current++ }
  }, [backendURL])
  const save = async (required: boolean, changeWord: boolean) => {
    writing.current = true; setPending(true)
    const id = ++sequence.current
    try {
      const response = await fetch(new URL('/api/wake-mode', backendURL), { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(changeWord ? { required, word, min_confidence: confidence, min_similarity: similarity } : { required }) })
      if (!response.ok) throw new Error(await response.text() || '待機モードを更新できませんでした')
      const next = await response.json() as WakeMode
      if (id === sequence.current) { setMode(next); setWord(next.word); setConfidence(next.min_confidence); setSimilarity(next.min_similarity); dirty.current = false; setError('') }
    } catch (err) { setError(err instanceof Error ? err.message : '更新できませんでした') }
    finally { writing.current = false; setPending(false) }
  }
  return (
    <div style={{ display: 'grid', gap: 4, fontSize: 12 }}>
      <label style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
        <input type="checkbox" checked={mode?.required ?? false} disabled={!mode || pending} onChange={event => void save(event.currentTarget.checked, false)} />呼びかけを必要にする
      </label>
      <span style={{ color: 'var(--live-muted)' }}>{!mode ? '確認中…' : !mode.required ? '呼びかけなしで会話できます' : `発話ごとに「${mode.word}」の呼びかけが必要です`}</span>
      <details><summary>呼びかけの設定</summary>
        <form onSubmit={event => { event.preventDefault(); if (mode) void save(mode.required, true) }} style={{ display: 'grid', gap: 6 }}>
          <label>呼びかけの言葉<input aria-label="呼びかけの言葉" maxLength={64} value={word} onChange={event => { setWord(event.currentTarget.value); dirty.current = true }} style={{ maxWidth: '100%', padding: 4, border: '1px solid var(--live-line)', borderRadius: 4, background: 'var(--live-panel-soft)', color: 'var(--live-text)' }} /></label>
          <label>STT信頼度の下限<input type="number" min="0.01" max="1" step="0.01" value={confidence} onChange={event => {setConfidence(Number(event.currentTarget.value)); dirty.current = true}} /></label>
          <label>類似度の下限<input type="number" min="0.01" max="1" step="0.01" value={similarity} onChange={event => {setSimilarity(Number(event.currentTarget.value)); dirty.current = true}} /></label>
          <button className="live-admin-btn" disabled={!mode || pending || !word.trim()}>保存</button>
          <span style={{ color: 'var(--live-muted)' }}>必須モードではローカルで認識・判定し、通過した発話のテキストだけを会話AIへ送ります。信頼度は正答率ではなくモデルの指標です。</span>
        </form>
      </details>
      {mode?.last && <span style={{color: 'var(--live-muted)'}}>直近の判定: {mode.last.failed ? '認識失敗' : mode.last.accepted ? '受付' : '無視'} / 信頼度 {mode.last.confidence.toFixed(2)} / 類似度 {mode.last.similarity.toFixed(2)}</span>}
      {error && <span role="alert">{error}</span>}
    </div>
  )
}
