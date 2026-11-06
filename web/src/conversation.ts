export interface ConversationMessage {
  type: string
  text: string
  generationId?: number
}

interface PlaybackSpeech {
  generationId: number
  text: string
}

// 入力文字起こしは応答より遅れて届くことがあるため、到着順ではなく同じ世代で組にする。
export function conversationBubble(messages: readonly ConversationMessage[], playback?: PlaybackSpeech) {
  const speech = messages.filter(message => message.type === 'user' || message.type === 'agent')
  let generation: number | undefined = playback?.generationId
  for (const message of speech) {
    if (Number.isSafeInteger(message.generationId)) {
      generation = Math.max(generation ?? 0, message.generationId!)
    }
  }
  const current = generation === undefined ? speech : speech.filter(message => message.generationId === generation)
  let user = ''
  for (const message of current) {
    if (message.type === 'user') user = message.text
  }
  // 全文の受信では吹き出しを更新せず、実際に再生を始めた区間だけを表示する。
  const assistant = playback && playback.generationId === generation ? playback.text : ''
  return { user, assistant, visible: Boolean(user || assistant) }
}
