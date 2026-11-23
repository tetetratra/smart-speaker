import { requestJSON } from '../../shared/backend'

// TTSSelection は選択中の読み上げ音声と、選べる読み上げ音声の一覧。
export type TTSSelection = {
  provider: string
  providers: { id: string; label: string; available: boolean; credit?: string }[]
}

export function getTTS(): Promise<TTSSelection> {
  return requestJSON('/api/tts', '読み上げ音声を取得できませんでした')
}

export function setTTS(provider: string): Promise<TTSSelection> {
  return requestJSON('/api/tts', '読み上げ音声を変更できませんでした', { method: 'PUT', body: { provider } })
}
