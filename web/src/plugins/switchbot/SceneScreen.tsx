import React, { useCallback, useEffect, useMemo, useState } from 'react'
import { PageLayout } from '../../shared/ui/PageLayout'
import { executeScene, listScenes, type Scene } from './api'
import './SceneScreen.css'

// SwitchBot のシーンを実行するリモコン画面。開いたときにシーン一覧を取得する。
export function SceneScreen({ onBack }: { onBack: () => void }) {
  const [scenes, setScenes] = useState<Scene[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [pendingSceneID, setPendingSceneID] = useState('')
  useEffect(() => {
    const controller = new AbortController()
    listScenes(controller.signal)
      .then(setScenes)
      .catch((err) => {
        if (!controller.signal.aborted) setError(err instanceof Error ? err.message : 'シーン一覧を取得できませんでした')
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })
    return () => controller.abort()
  }, [])
  const sortedScenes = useMemo(() => {
    return [...scenes].sort((a, b) => a.name.localeCompare(b.name, 'ja'))
  }, [scenes])
  const submitScene = useCallback(async (scene: Scene) => {
    setPendingSceneID(scene.id)
    setError('')
    try {
      await executeScene(scene.id)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'シーンを実行できませんでした')
    } finally {
      setPendingSceneID('')
    }
  }, [])
  return (
    <PageLayout onBack={onBack}>
      {error && <div className="error-banner">{error}</div>}
      <div className="panel">
        {loading && <div className="scene-message">シーンを読み込み中です</div>}
        {!loading && !error && scenes.length === 0 && <div className="scene-message">利用できるシーンはありません</div>}
        <div className="switchbot-scene-grid">
          {sortedScenes.map((scene) => {
            const pending = pendingSceneID === scene.id
            return (
              <button
                key={scene.id}
                onClick={() => void submitScene(scene)}
                disabled={pendingSceneID !== ''}
                className={`switchbot-scene-button${pending ? ' pending' : ''}`}
              >
                {pending ? '実行中' : scene.name}
              </button>
            )
          })}
        </div>
      </div>
    </PageLayout>
  )
}
