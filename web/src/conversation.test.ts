import test from 'node:test'
import assert from 'node:assert/strict'
import { conversationBubble } from './conversation.ts'

test('AIの返答を待たずユーザーの文字起こしだけでも表示する', () => {
 assert.deepEqual(conversationBubble([{type:'user',text:'こんにちは',generationId:1}]), {user:'こんにちは',assistant:'',visible:true})
})
test('ユーザーの文字起こしより先に届いたAIの会話を表示し、後から同じ世代のユーザー発話を組にする', () => {
 const messages=[{type:'agent',text:'こんにちは。',generationId:2}]
 assert.deepEqual(conversationBubble(messages),{user:'',assistant:'こんにちは。',visible:true})
 assert.deepEqual(conversationBubble([...messages,{type:'user',text:'おはよう',generationId:2}]),{user:'おはよう',assistant:'こんにちは。',visible:true})
})
test('遅れて届く旧世代の文字起こしやツール結果で現在の吹き出しを上書きしない', () => {
 assert.deepEqual(conversationBubble([
  {type:'user',text:'昔の会話',generationId:1},
  {type:'agent',text:'昔の返答',generationId:1},
  {type:'user',text:'今の会話',generationId:2},
  {type:'tool_result',text:'実行結果',generationId:99},
  {type:'user',text:'遅延した昔の文字起こし',generationId:1},
 ]),{user:'今の会話',assistant:'',visible:true})
 assert.equal(conversationBubble([]).visible,false)
})
