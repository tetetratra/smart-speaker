import { requestJSON } from '../../shared/backend'

export type Scene = { id: string; name: string }

export async function listScenes(signal?: AbortSignal): Promise<Scene[]> {
  const data = await requestJSON<{ scenes: Scene[] }>('/api/switchbot/scenes', 'シーン一覧を取得できませんでした', { signal })
  if (!Array.isArray(data?.scenes)) throw new Error('シーン一覧の形式が不正です')
  return data.scenes.map((scene) => ({ id: String(scene.id), name: String(scene.name) }))
}

export async function executeScene(id: string): Promise<void> {
  await requestJSON(`/api/switchbot/scenes/${encodeURIComponent(id)}/execute`, 'シーンを実行できませんでした', { method: 'POST' })
}
