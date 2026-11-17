// ConversationMessage はユーザーの発話。AI の発話は届いた全文ではなく、再生を始めた区間（PlaybackSpeech）から出す。
export interface ConversationMessage {
  type: 'user'
  text: string
  generationId?: number
}

interface PlaybackSpeech {
  generationId: number
  text: string
}

// 入力文字起こしは応答より遅れて届くことがあるため、到着順ではなく同じ世代で組にする。
export function conversationBubble(messages: readonly ConversationMessage[], playback?: PlaybackSpeech) {
  const speech = messages
  let generation: number | undefined = playback?.generationId
  for (const message of speech) {
    if (Number.isSafeInteger(message.generationId)) {
      generation = Math.max(generation ?? 0, message.generationId!)
    }
  }
  const current = generation === undefined ? speech : speech.filter(message => message.generationId === generation)
  const user = current.at(-1)?.text ?? ''
  // AI の側は、実際に再生を始めた区間だけを表示する。
  const assistant = playback && playback.generationId === generation ? playback.text : ''
  return { user, assistant, visible: Boolean(user || assistant) }
}
