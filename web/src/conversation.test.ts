import test from 'node:test'
import assert from 'node:assert/strict'
import { conversationBubble } from './conversation.ts'

test('AIの返答を待たずユーザーの文字起こしだけでも表示する', () => {
 assert.deepEqual(conversationBubble([{type:'user',text:'こんにちは',generationId:1}]), {user:'こんにちは',assistant:'',visible:true})
})
test('遅れて届く旧世代の文字起こしで現在の吹き出しを上書きしない', () => {
 assert.deepEqual(conversationBubble([
  {type:'user',text:'昔の会話',generationId:1},
  {type:'user',text:'今の会話',generationId:2},
  {type:'user',text:'遅延した昔の文字起こし',generationId:1},
 ]),{user:'今の会話',assistant:'',visible:true})
 assert.equal(conversationBubble([]).visible,false)
})

test('再生する区間ごとに置き換え、遅れた文字起こしも組にする', () => {
 const first={generationId:2,text:'最初の読み上げ区間。'}
 const second={generationId:2,text:'次の読み上げ区間。'}
 assert.deepEqual(conversationBubble([],first),{user:'',assistant:first.text,visible:true})
 assert.deepEqual(conversationBubble([{type:'user',text:'質問',generationId:2}],second),{user:'質問',assistant:second.text,visible:true})
 assert.equal(conversationBubble([{type:'user',text:'次の質問',generationId:3}],second).assistant,'')
})
