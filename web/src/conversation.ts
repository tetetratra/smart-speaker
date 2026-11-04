export interface ConversationMessage {
  type: string
  text: string
  generationId?: number
}

// 入力文字起こしは応答より遅れて届くことがあるため、到着順ではなく同じ世代で組にする。
export function conversationBubble(messages: readonly ConversationMessage[]) {
  const speech = messages.filter(message => message.type === 'user' || message.type === 'agent')
  let generation: number | undefined
  for (const message of speech) {
    if (Number.isSafeInteger(message.generationId)) {
      generation = Math.max(generation ?? 0, message.generationId!)
    }
  }
  const current = generation === undefined ? speech : speech.filter(message => message.generationId === generation)
  let user = '', assistant = ''
  for (const message of current) {
    if (message.type === 'user') user = message.text
    else assistant = message.text
  }
  return { user, assistant, visible: Boolean(user || assistant) }
}
