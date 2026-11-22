import type { ConversationRecord } from './protocol.ts'

// 画面に出す会話の状態を、受信メッセージと接続の出来事から作る純粋関数。
// 世代番号（サーバーが応答ごとに振る番号。割り込まれると次の番号になる）の新旧は、ここ 1 か所で判定する。

export type ConnectionStatus = 'offline' | 'connecting' | 'online'
// マイクの状態。detecting は話し声を検出している間。
export type MicStatus = 'idle' | 'detecting'
// 音声認識の状態。
export type RecognitionStatus = 'stopped' | 'connecting' | 'idle' | 'recognizing' | 'finalizing' | 'done' | 'error'

// PlaybackSpeech はサーバーが送り始めた読み上げの区間。
export type PlaybackSpeech = {
  generationId: number
  segmentId: string
  text: string
  // 区間全体の再生時間。吹き出しを再生終了に合わせてスクロールするのに使う。
  durationMs?: number
}

// Utterance は吹き出しに出すユーザーの発話。世代のない発話は generationId が undefined。
export type Utterance = { text: string; generationId?: number }

export type SessionState = {
  connection: ConnectionStatus
  mic: MicStatus
  recognition: RecognitionStatus
  // 受け取った中で最も新しい世代。接続ごとに 0 から数え直す（サーバーの再起動で番号が戻るため）。
  generation: number
  utterances: Utterance[]
  // 今再生中の読み上げ区間。
  speech?: PlaybackSpeech
  // 読み上げ中かどうか。吹き出しを光らせるのに使う。
  speaking: boolean
  // ホワイトボードに出す記録。接続し直しても消さない（再接続時にサーバーが送り直し、同じ ID は上書きされる）。
  records: Map<string, ConversationRecord>
}

// SessionEvent は接続の出来事と、受け取ったメッセージ。
export type SessionEvent =
  // 接続を試み始めた（手動・自動再接続とも）。
  | { type: 'connecting' }
  // ソケットが開いた。
  | { type: 'open' }
  // マイクを取り、WebRTC の接続を始められた。
  | { type: 'mic_ready' }
  // マイクを取れなかった、または WebRTC の接続を始められなかった。ソケットは開いたまま。
  | { type: 'mic_failed' }
  // ソケットが閉じた（自動再接続を待つ）。
  | { type: 'closed' }
  // 手動で切断した。
  | { type: 'disconnected' }
  | { type: 'message'; message: any }

export function initialSessionState(): SessionState {
  return {
    connection: 'offline',
    mic: 'idle',
    recognition: 'stopped',
    generation: 0,
    utterances: [],
    speech: undefined,
    speaking: false,
    records: new Map(),
  }
}

export function sessionReducer(state: SessionState, event: SessionEvent): SessionState {
  switch (event.type) {
    case 'connecting':
      return { ...state, connection: 'connecting', generation: 0, utterances: [], speech: undefined, speaking: false }
    case 'open':
      return { ...state, connection: 'online' }
    case 'mic_ready':
      return { ...state, mic: 'idle', recognition: 'idle' }
    case 'mic_failed':
      return { ...state, mic: 'idle', recognition: 'stopped', speaking: false }
    case 'closed':
      return { ...state, connection: 'offline', mic: 'idle', recognition: 'stopped', speaking: false }
    case 'disconnected':
      return { ...state, connection: 'offline', mic: 'idle', recognition: 'stopped', utterances: [], speech: undefined, speaking: false }
    case 'message':
      return reduceMessage(state, event.message)
  }
}

function reduceMessage(previous: SessionState, message: any): SessionState {
  if (!message || typeof message !== 'object') return previous
  const generation = Number.isSafeInteger(message.generation_id) ? message.generation_id as number : undefined
  // 届いたメッセージの世代が新しければ、今の世代を進める。古い世代かどうかは、進める前の世代と比べる。
  const state = generation !== undefined && generation > previous.generation ? { ...previous, generation } : previous
  switch (message.type) {
    case 'playback_progress': {
      if (generation === undefined || generation < previous.generation) return state
      const speaking = !message.end
      // 送り始めの知らせで表示する。実際の再生とは受信バッファの分だけずれる。
      if (speaking && message.text) {
        const speech = { generationId: generation, segmentId: String(message.segment_id ?? ''), text: String(message.text), durationMs: message.duration_ms }
        return { ...state, speaking, speech }
      }
      return { ...state, speaking }
    }
    case 'playback_stop': {
      if (generation === undefined || generation < previous.generation) return state
      // 止めた世代より古い区間を消す。サーバーが残りを送らないので、受信済みの短い残りは流し切る。
      const speech = state.speech && state.speech.generationId < generation ? undefined : state.speech
      return { ...state, speaking: false, speech }
    }
    case 'realtime_status': {
      const recognition: RecognitionStatus = message.status === 'connected' ? 'idle' : message.status === 'error' ? 'error' : 'connecting'
      return { ...state, recognition }
    }
    case 'speech_start':
      return { ...state, speaking: false, mic: 'detecting', recognition: 'recognizing' }
    case 'speech_end':
      return { ...state, mic: 'idle', recognition: 'finalizing' }
    case 'conversation_records': {
      if (!Array.isArray(message.records)) return state
      return reduceRecords(state, message.records as ConversationRecord[])
    }
    default:
      return state
  }
}

// 画面に届いた記録を、ユーザーの発話（吹き出し）とそれ以外（ホワイトボード）に分けて足す。
function reduceRecords(state: SessionState, records: ConversationRecord[]): SessionState {
  const { utterances, others } = splitRecords(records)
  let next = state
  for (const utterance of utterances) {
    const generation = utterance.generationId !== undefined && utterance.generationId > next.generation ? utterance.generationId : next.generation
    // 今の世代の発話が届いたら、その発話の認識は終わっている。
    const current = utterance.generationId === undefined || utterance.generationId === generation
    next = {
      ...next,
      generation,
      utterances: [...next.utterances, utterance],
      ...(current ? { speaking: false, recognition: 'done' as const, mic: 'idle' as const } : {}),
    }
  }
  if (others.length > 0) {
    const merged = new Map(next.records)
    // 同じ ID が届いても重複させない。
    for (const record of others) merged.set(record.id, record)
    next = { ...next, records: merged }
  }
  return next
}

// splitRecords は画面に届いた記録を、どこに出すかで分ける。ユーザーの発話は吹き出し、それ以外はホワイトボードに出す。
// 世代のない記録は generation_id が 0 で届くので、世代なし（undefined）として扱う。
export function splitRecords(records: ConversationRecord[]) {
  const utterances: Utterance[] = []
  const others: ConversationRecord[] = []
  for (const record of records) {
    if (record.role !== 'user') {
      others.push(record)
      continue
    }
    const generation = record.generation_id
    utterances.push({ text: record.text, generationId: Number.isSafeInteger(generation) && generation! > 0 ? generation : undefined })
  }
  return { utterances, others }
}

export type Bubble = {
  user: string
  assistant: string
  // 吹き出しに出している読み上げ区間。区間が変わるたびに再生時間に合わせて自動スクロールする。
  speech?: PlaybackSpeech
  visible: boolean
}

// bubbleOf は吹き出しの中身を求める。今の世代のユーザーの発話と、今の世代で再生を始めた読み上げ区間を組にする。
// 入力の文字起こしは応答より遅れて届くことがあるため、到着順ではなく同じ世代で組にする。
export function bubbleOf(state: SessionState): Bubble {
  const current = state.utterances.filter(utterance => (utterance.generationId ?? 0) === state.generation)
  const user = current.at(-1)?.text ?? ''
  const speech = state.speech && state.speech.generationId === state.generation ? state.speech : undefined
  const assistant = speech?.text ?? ''
  return { user, assistant, speech, visible: Boolean(user || assistant) }
}
