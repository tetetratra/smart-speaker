import React, { useCallback, useEffect, useRef, useState } from 'react'
import { getTTS, setTTS, type TTSSelection } from './api'
import './TTSSelect.css'

// 読み上げ音声を選ぶ。マウントしたときに選択中のものを取得する。
export function TTSSelect() {
  const [selection, setSelection] = useState<TTSSelection | null>(null)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')
  const requestID = useRef(0)
  // 取得と変更が重なったときは、最後に始めたものの結果だけを使う。
  const request = useCallback(async (provider?: string) => {
    const id = ++requestID.current
    setPending(true)
    setError('')
    try {
      const next = provider === undefined ? await getTTS() : await setTTS(provider)
      if (id === requestID.current) setSelection(next)
    } catch (err) {
      if (id === requestID.current) setError(err instanceof Error ? err.message : '読み上げ音声を変更できませんでした')
    } finally {
      if (id === requestID.current) setPending(false)
    }
  }, [])
  useEffect(() => {
    void request()
    return () => { requestID.current++ }
  }, [request])
  // VOICEVOX などの利用規約が求めるクレジット表記。選択中のTTSのものだけ出す
  const credit = selection?.providers.find((provider) => provider.id === selection.provider)?.credit
  return (
    <div className="tts-select">
      <label className="tts-select-row">
        <select
          aria-label="読み上げ音声"
          value={selection?.provider ?? ''}
          disabled={pending || !selection}
          onChange={(event) => void request(event.currentTarget.value)}
        >
          {!selection && <option value="">{pending ? '読み込み中' : '取得できません'}</option>}
          {selection?.providers.map((provider) => (
            <option key={provider.id} value={provider.id} disabled={!provider.available}>
              {provider.label}{provider.available ? '' : '（未設定）'}
            </option>
          ))}
        </select>
        {error && <button className="live-admin-btn" onClick={() => void request()} disabled={pending}>再取得</button>}
      </label>
      {credit && <span className="tts-select-credit">{credit}</span>}
      {error && <span role="alert">{error}</span>}
    </div>
  )
}
