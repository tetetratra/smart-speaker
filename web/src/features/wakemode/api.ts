import { requestJSON } from '../../shared/backend'

// requireWake は、呼びかけの言葉を聞くまで会話を始めないようにする。
export async function requireWake(): Promise<void> {
  await requestJSON('/api/wake-mode', '呼びかけを必須にできませんでした', { method: 'PUT', body: { required: true } })
}

// wake は、呼びかけなしで話せる会話中の状態にする。
export async function wake(): Promise<void> {
  await requestJSON('/api/wake-mode/wake', '呼びかけ中にできませんでした', { method: 'POST' })
}
