// /ws/chat でサーバーから届くメッセージと、会話の記録の型。
// 値の確認（配列か、整数か）は、受け取った側（sessionReducer）で今と同じ最低限だけ行う。

// ConversationRecord はサーバーが送る会話履歴の記録。ユーザーの発話は吹き出し、それ以外はホワイトボードに出す。
export type ConversationRecord = {
  id: string
  role: string
  text: string
  source: string
  // 世代のない記録は 0 で届く。
  generation_id?: number
  title?: string
  external?: boolean
  call_id?: string
  arguments?: string
  output?: string
  error?: string
  created_at: string
}

// サーバーが読み上げの区間を送り始めた（end なら送り終えた）ことを知らせる。
export type PlaybackProgressMessage = {
  type: 'playback_progress'
  generation_id: number
  segment_id: string
  text?: string
  end?: boolean
  // 区間全体の再生時間。
  duration_ms?: number
}

// その世代より古い読み上げを止めたことを知らせる。
export type PlaybackStopMessage = { type: 'playback_stop'; generation_id: number }

// 音声認識（OpenAI Realtime）との接続状態。
export type RealtimeStatusMessage = { type: 'realtime_status'; status: string; message?: string }

export type SpeechStartMessage = { type: 'speech_start' }
export type SpeechEndMessage = { type: 'speech_end' }
export type ConversationRecordsMessage = { type: 'conversation_records'; records: ConversationRecord[] }

// WebRTC の接続に使うメッセージ。画面の状態には関わらず、peer に渡す。
export type WebRTCSignalMessage =
  | { type: 'webrtc.answer'; sdp?: string }
  | { type: 'webrtc.ice'; candidate?: RTCIceCandidateInit }

export type ServerMessage =
  | PlaybackProgressMessage
  | PlaybackStopMessage
  | RealtimeStatusMessage
  | SpeechStartMessage
  | SpeechEndMessage
  | ConversationRecordsMessage
  | WebRTCSignalMessage

export function isWebRTCSignal(message: { type?: unknown }): message is WebRTCSignalMessage {
  return message.type === 'webrtc.answer' || message.type === 'webrtc.ice'
}
