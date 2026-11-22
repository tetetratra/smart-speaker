import test from 'node:test'
import assert from 'node:assert/strict'
import { bubbleOf, initialSessionState, sessionReducer, splitRecords, type SessionEvent, type SessionState } from './sessionReducer.ts'
import type { ConversationRecord } from './protocol.ts'

const message = (value: any): SessionEvent => ({ type: 'message', message: value })
const run = (...events: SessionEvent[]) => events.reduce(sessionReducer, initialSessionState())
const record = (id: string, role: string, text: string, generation = 0): ConversationRecord =>
  ({ id, role, text, source: role === 'user' ? 'user' : 'set_whiteboard', generation_id: generation, created_at: '2026-11-22T00:00:00Z' })
const userRecords = (...items: [string, number][]) =>
  message({ type: 'conversation_records', records: items.map(([text, generation], i) => record(`u${i}-${text}`, 'user', text, generation)) })
const progress = (generation: number, text?: string, end = false) =>
  message({ type: 'playback_progress', generation_id: generation, segment_id: `s-${text ?? ''}`, text, end })

test('AIの返答を待たずユーザーの文字起こしだけでも表示する', () => {
  assert.deepEqual(bubbleOf(run(userRecords(['こんにちは', 1]))), { user: 'こんにちは', assistant: '', speech: undefined, visible: true })
})

test('遅れて届く旧世代の文字起こしで現在の吹き出しを上書きしない', () => {
  const state = run(userRecords(['昔の会話', 1]), userRecords(['今の会話', 2]), userRecords(['遅延した昔の文字起こし', 1]))
  assert.equal(bubbleOf(state).user, '今の会話')
  assert.equal(bubbleOf(initialSessionState()).visible, false)
})

test('再生する区間ごとに置き換え、遅れた文字起こしも組にする', () => {
  let state = run(progress(2, '最初の読み上げ区間。'))
  assert.deepEqual([bubbleOf(state).user, bubbleOf(state).assistant], ['', '最初の読み上げ区間。'])
  state = [progress(2, '次の読み上げ区間。'), userRecords(['質問', 2])].reduce(sessionReducer, state)
  assert.deepEqual([bubbleOf(state).user, bubbleOf(state).assistant], ['質問', '次の読み上げ区間。'])
  state = sessionReducer(state, userRecords(['次の質問', 3]))
  assert.deepEqual([bubbleOf(state).user, bubbleOf(state).assistant], ['次の質問', ''])
})

test('世代のない発話は、ほかに世代がなければそのまま出す', () => {
  assert.equal(bubbleOf(run(userRecords(['一つ目', 0]), userRecords(['二つ目', 0]))).user, '二つ目')
})

test('古い世代の読み上げは捨て、止めた世代より古い区間を消す', () => {
  const shown: string[] = []
  const speaking: boolean[] = []
  let state = initialSessionState()
  const step = (event: SessionEvent) => {
    const before = state
    state = sessionReducer(state, event)
    if (state.speech !== before.speech && state.speech) shown.push(state.speech.text)
    if (state.speaking !== before.speaking) speaking.push(state.speaking)
  }
  step(progress(1, '旧応答'))
  step(message({ type: 'playback_stop', generation_id: 2 }))
  assert.equal(state.speech, undefined)
  step(progress(1, '遅れた旧応答'))
  step(progress(2, '新応答'))
  step(message({ type: 'playback_stop', generation_id: 1 }))
  step(progress(1, undefined, true))
  assert.deepEqual(shown, ['旧応答', '新応答'])
  assert.deepEqual(speaking, [true, false, true])
  assert.equal(state.speech?.text, '新応答')
  step(progress(2, undefined, true))
  assert.equal(state.speaking, false)
})

test('同じ世代の playback_stop でも読み上げ中の印を消す', () => {
  // サーバーは新しい世代を始めるとき、その世代の playback_stop を送る。ほかのメッセージで先に世代が進んでいても止める。
  const state = run(progress(1, '応答'), message({ type: 'speech_start', generation_id: 2 }), progress(1, undefined), message({ type: 'playback_stop', generation_id: 2 }))
  assert.equal(state.speaking, false)
  assert.equal(state.speech, undefined)
})

test('今の世代の発話が届いたら認識を完了にする', () => {
  let state = run({ type: 'open' }, { type: 'mic_ready' }, message({ type: 'speech_start' }))
  assert.deepEqual([state.mic, state.recognition], ['detecting', 'recognizing'])
  state = sessionReducer(state, message({ type: 'speech_end' }))
  assert.deepEqual([state.mic, state.recognition], ['idle', 'finalizing'])
  state = sessionReducer(state, userRecords(['こんにちは', 1]))
  assert.deepEqual([state.mic, state.recognition, state.generation], ['idle', 'done', 1])
  state = sessionReducer(state, message({ type: 'realtime_status', status: 'error' }))
  assert.equal(state.recognition, 'error')
})

test('ユーザーの発話以外の記録はホワイトボードに足し、同じ ID は重複させない', () => {
  const board = message({ type: 'conversation_records', records: [record('t1', 'tool', '', 1), record('u1', 'user', '書いて', 1)] })
  const state = run(board, board)
  assert.deepEqual([...state.records.keys()], ['t1'])
  assert.equal(state.utterances.length, 2)
})

test('接続の出来事ごとに、消すものと残すものを分ける', () => {
  const talking = (): SessionState => run(
    { type: 'connecting' }, { type: 'open' }, { type: 'mic_ready' },
    message({ type: 'conversation_records', records: [record('t1', 'tool', '', 3), record('u1', 'user', '質問', 3)] }),
    progress(3, '応答'),
  )
  const kept = (state: SessionState) => ({
    connection: state.connection, mic: state.mic, recognition: state.recognition, generation: state.generation,
    bubble: bubbleOf(state).visible, speaking: state.speaking, board: state.records.size,
  })
  assert.deepEqual(kept(talking()), { connection: 'online', mic: 'idle', recognition: 'done', generation: 3, bubble: true, speaking: true, board: 1 })
  // 接続を試み始めた: 世代・発話・再生中の区間を消し、ホワイトボードの記録は残す。
  assert.deepEqual(kept(sessionReducer(talking(), { type: 'connecting' })), { connection: 'connecting', mic: 'idle', recognition: 'done', generation: 0, bubble: false, speaking: false, board: 1 })
  // マイクを取れなかった: 読み上げ中の印だけを消す。
  assert.deepEqual(kept(sessionReducer(talking(), { type: 'mic_failed' })), { connection: 'online', mic: 'idle', recognition: 'stopped', generation: 3, bubble: true, speaking: false, board: 1 })
  // ソケットが閉じた: 読み上げ中の印を消し、吹き出しは残す。
  assert.deepEqual(kept(sessionReducer(talking(), { type: 'closed' })), { connection: 'offline', mic: 'idle', recognition: 'stopped', generation: 3, bubble: true, speaking: false, board: 1 })
  // 手動で切断した: 吹き出しと読み上げ中の印を消す。
  assert.deepEqual(kept(sessionReducer(talking(), { type: 'disconnected' })), { connection: 'offline', mic: 'idle', recognition: 'stopped', generation: 3, bubble: false, speaking: false, board: 1 })
})

test('届いた記録を、吹き出しに出すユーザーの発話と、ホワイトボードに出すそれ以外に分ける', () => {
  const item = (id: string, role: string, generation_id?: number): ConversationRecord => ({ id, role, text: `本文${id}`, source: 'realtime', generation_id, created_at: '2026-11-18T00:00:00.000Z' })
  const { utterances, others } = splitRecords([item('1', 'user', 3), item('2', 'tool', 3), item('3', 'user', 0), item('4', 'background')])
  assert.deepEqual(utterances, [{ text: '本文1', generationId: 3 }, { text: '本文3', generationId: undefined }])
  assert.deepEqual(others.map(r => r.id), ['2', '4'])
})

test('割り込みで新しい世代の playback_stop が届いたら、その世代の発話が届くまで吹き出しを隠す', () => {
  // 世代は playback_stop でも進む。前の世代の発話を出し続けず、新しい世代の文字起こしか読み上げが届いたら出す。
  let state = run(userRecords(['前の質問', 1]), progress(1, '前の応答'))
  assert.equal(bubbleOf(state).visible, true)
  state = sessionReducer(state, message({ type: 'playback_stop', generation_id: 2 }))
  assert.equal(bubbleOf(state).visible, false)
  state = sessionReducer(state, userRecords(['割り込んだ質問', 2]))
  assert.deepEqual([bubbleOf(state).user, bubbleOf(state).assistant], ['割り込んだ質問', ''])
})
