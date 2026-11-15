import { useCallback, useEffect, useMemo, useState } from 'react'

type SwitchBotSceneItem = {
  id: string
  name: string
}
type SwitchBotSceneListResponse = { scenes: SwitchBotSceneItem[] }

function normalizeSwitchBotSceneItem(scene: SwitchBotSceneItem): SwitchBotSceneItem {
  return {
    id: String(scene.id),
    name: String(scene.name),
  }
}

// SwitchBot のシーンを実行するリモコン画面。表示されたときにシーン一覧を取得する。
export function SwitchBotSceneView(props: {
  visible: boolean
  backendURL: URL
  goApp: () => void
}) {
  const { visible, backendURL, goApp } = props
  const [scenes, setScenes] = useState<SwitchBotSceneItem[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [pendingSceneID, setPendingSceneID] = useState('')
  useEffect(() => {
    if (!visible) return
    const controller = new AbortController()
    const { signal } = controller
    setLoading(true)
    setError('')
    fetch(new URL('/api/switchbot/scenes', backendURL).toString(), { signal })
      .then(async (resp) => {
        if (!resp.ok) {
          throw new Error(`シーン一覧を取得できませんでした (${resp.status})`)
        }
        return await resp.json() as SwitchBotSceneListResponse
      })
      .then((data) => {
        if (!Array.isArray(data.scenes)) {
          throw new Error('シーン一覧の形式が不正です')
        }
        setScenes(data.scenes.map(normalizeSwitchBotSceneItem))
      })
      .catch((err) => {
        if (signal.aborted) return
        setError(err instanceof Error ? err.message : 'シーン一覧を取得できませんでした')
      })
      .finally(() => {
        if (!signal.aborted) {
          setLoading(false)
        }
      })
    return () => controller.abort()
  }, [backendURL, visible])
  const onExecute = useCallback(async (scene: SwitchBotSceneItem) => {
    try {
      setError('')
      const resp = await fetch(new URL(`/api/switchbot/scenes/${encodeURIComponent(scene.id)}/execute`, backendURL).toString(), { method: 'POST' })
      if (!resp.ok) {
        throw new Error(`シーンを実行できませんでした (${resp.status})`)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'シーンを実行できませんでした')
      throw err
    }
  }, [backendURL])
  const sortedScenes = useMemo(() => {
    return [...scenes].sort((a, b) => a.name.localeCompare(b.name, 'ja'))
  }, [scenes])
  const submitScene = useCallback(async (scene: SwitchBotSceneItem) => {
    setPendingSceneID(scene.id)
    try {
      await onExecute(scene)
    } finally {
      setPendingSceneID('')
    }
  }, [onExecute])
  return (
    <div
      style={{
        display: visible ? 'flex' : 'none',
        flexDirection: 'column',
        gap: 12,
        padding: 12,
        height: '100vh',
        minHeight: 0,
        overflow: 'hidden',
        boxSizing: 'border-box',
      }}
    >
      <style>{`
        .switchbot-scene-grid {
          display: grid;
          grid-template-columns: repeat(2, minmax(0, 1fr));
          gap: 10px;
        }
        .switchbot-scene-button {
          min-width: 0;
          min-height: 42px;
          border-radius: 8px;
          border: 1px solid var(--live-line-strong);
          padding: 4px 8px;
          font-size: 21px;
          font-weight: 800;
          line-height: 1.2;
          white-space: normal;
          overflow-wrap: anywhere;
          word-break: break-word;
          box-shadow: 0 1px 2px rgba(15, 23, 42, 0.04);
        }
        @media (orientation: portrait) {
          .switchbot-scene-grid {
            grid-template-columns: minmax(0, 1fr);
          }
        }
      `}</style>
      {error && (
        <div
          style={{
            border: '1px solid var(--ui-danger-line)',
            borderRadius: 8,
            background: 'var(--ui-danger-bg)',
            color: 'var(--ui-danger-text)',
            padding: '10px 12px',
            fontSize: 13,
            lineHeight: 1.4,
            fontWeight: 700,
          }}
        >
          {error}
        </div>
      )}
      <div
        style={{
          flex: '1 1 auto',
          minHeight: 0,
          overflow: 'auto',
          border: '2px solid var(--live-line)',
          borderRadius: 10,
          background: 'var(--live-panel-soft)',
          padding: 12,
        }}
      >
        {loading && (
          <div style={{ color: 'var(--live-muted)', fontSize: 14, fontWeight: 700 }}>シーンを読み込み中です</div>
        )}
        {!loading && !error && scenes.length === 0 && (
          <div style={{ color: 'var(--live-muted)', fontSize: 14, fontWeight: 700 }}>利用できるシーンはありません</div>
        )}
        <div
          className="switchbot-scene-grid"
        >
          {sortedScenes.map((scene) => {
            const pending = pendingSceneID === scene.id
            return (
              <button
                key={scene.id}
                onClick={() => void submitScene(scene)}
                disabled={pendingSceneID !== ''}
                className="switchbot-scene-button"
                style={{
                  background: pending ? 'var(--ui-accent-soft)' : 'var(--live-panel)',
                  color: pending ? 'var(--ui-accent-soft-text)' : 'var(--live-text)',
                  cursor: pendingSceneID !== '' ? 'not-allowed' : 'pointer',
                }}
              >
                {pending ? '実行中' : scene.name}
              </button>
            )
          })}
        </div>
      </div>
      <button
        onClick={goApp}
        style={{
          alignSelf: 'flex-start',
          borderRadius: 8,
          border: '1px solid var(--live-line-strong)',
          background: 'var(--live-panel)',
          color: 'var(--live-text)',
          padding: '10px 14px',
          fontSize: 14,
          fontWeight: 700,
          cursor: 'pointer',
        }}
      >
        app画面
      </button>
    </div>
  )
}
