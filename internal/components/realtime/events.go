package realtime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/tetetratra/smart-speaker/internal/types"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type serverEvent struct {
	Type                                     string `json:"type"`
	ItemID                                   string `json:"item_id"`
	ResponseID                               string `json:"response_id"`
	PreviousItemID                           string `json:"previous_item_id"`
	Delta, Text, Transcript, Name, Arguments string
	CallID                                   string `json:"call_id"`
	Item                                     struct {
		ID, Type, Role, Text string
		// Phase は、応答の item が前置き（commentary）か最終回答（final_answer）か。
		Phase string `json:"phase"`
	} `json:"item"`
	Response struct {
		ID, Status string
		Metadata   map[string]string
		Output     []struct{ ID, Type, Text string }
		Usage      *usage
	} `json:"response"`
	Error struct{ Code, Message string } `json:"error"`
}

func (s *stage) serverEvent(ctx context.Context, data []byte) {
	var e serverEvent
	if json.Unmarshal(data, &e) != nil {
		return
	}
	if s.refreshWakeMode(ctx) {
		return
	}
	s.refreshWakeConversation()
	// 呼びかけ前にクラウド側へ残った音声イベントは、呼びかけ判定には使わない。
	if s.wakeGated {
		switch e.Type {
		case "input_audio_buffer.committed":
			if e.ItemID != "" {
				s.transport.send(map[string]any{"type": "conversation.item.delete", "item_id": e.ItemID})
			}
			return
		case "input_audio_buffer.speech_started", "input_audio_buffer.speech_stopped", "conversation.item.input_audio_transcription.completed", "conversation.item.input_audio_transcription.failed":
			return
		}
	}
	switch e.Type {
	case "session.updated":
		if s.ready {
			return
		}
		s.ready = true
		s.started = time.Now()
		s.connecting = false
		// 振り分けが Realtime と同じ範囲（今のセッションのやりとり）を読めるよう、セッションの始まりを履歴に残す。画面には出さない。
		s.saveRecord(ctx, types.ConversationRecord{ID: fmt.Sprintf("%d/session", s.epoch), Role: types.RoleSystem, Text: "[音声AIの会話セッションを開始]", Source: types.SessionStartSource, CreatedAt: s.started}, true)
		for _, pcm := range s.pendingAudio {
			if s.wakeGated {
				break
			}
			s.transport.send(map[string]any{"type": "input_audio_buffer.append", "audio": encodeAudio(pcm)})
		}
		s.pendingAudio = nil
		pendingWake := s.pendingWake
		s.pendingWake = nil
		for _, utterance := range pendingWake {
			s.localWake(ctx, utterance)
		}
		s.status(ctx, "connected", "")
		s.flushExternal()
		s.maybeRespond(ctx)
	case "input_audio_buffer.speech_started":
		// 呼びかけ後の会話中は、ユーザーの発話ごとに会話中の時間を数え直す。
		if s.cfg.WakeMode != nil {
			s.cfg.WakeMode.Touch(time.Now())
		}
		s.awaitingInputCommit = e.ItemID
		s.speaking = true
		s.lastActivity = time.Now()
		s.lastTalk = s.lastActivity
		previous := s.cfg.Generation.Current()
		change, ok := s.controller.Start(fmt.Sprintf("%d/%s", s.epoch, e.ItemID))
		if !ok {
			return
		}
		s.recordInterruption(ctx, previous)
		change.ItemID = e.ItemID
		s.userGenerations[e.ItemID] = change.GenerationID
		s.emit(ctx, types.EventGenerationChanged{GenerationChanged: change})
		s.emit(ctx, types.EventSpeechStart{SpeechEvent: types.SpeechEvent{Source: "realtime-vad", CapturedAt: time.Now()}})
	case "input_audio_buffer.speech_stopped":
		s.speaking = false
		s.lastActivity = time.Now()
		s.emit(ctx, types.EventSpeechEnd{SpeechEvent: types.SpeechEvent{Source: "realtime-vad", CapturedAt: time.Now()}})
	case "input_audio_buffer.committed":
		if s.awaitingInputCommit == e.ItemID {
			s.awaitingInputCommit = ""
		}
		// 応答はここでは始めない。文字起こしが確定したら会話の流れが振り分け、応答依頼として届く。
		s.maybeRespond(ctx)
	case "conversation.item.input_audio_transcription.completed":
		gen, ok := s.userGenerations[e.ItemID]
		if !ok {
			return
		}
		s.record(ctx, e.ItemID, types.RoleUser, e.Transcript, gen)
	case "conversation.item.input_audio_transcription.failed":
		s.status(ctx, "error", "入力の文字起こしに失敗しました。音声による会話は継続します。")
	case "conversation.item.added", "conversation.item.created":
		s.previous[e.Item.ID] = e.PreviousItemID
		delete(s.repairs, e.Item.ID)
		s.maybeRespond(ctx)
	case "conversation.item.deleted":
		delete(s.repairs, e.ItemID)
		s.maybeRespond(ctx)
	case "response.created":
		gen64, err := strconv.ParseUint(e.Response.Metadata["generation"], 10, 64)
		if err != nil {
			s.status(ctx, "error", "応答の世代情報を確認できませんでした")
			return
		}
		s.responses[e.Response.ID] = &response{generation: types.GenerationID(gen64), items: map[string]bool{}}
		s.active = e.Response.ID
		s.lastTalk = time.Now()
		used := []string{}
		for id := range s.externalRequested {
			used = append(used, id)
			delete(s.externalPending, id)
			delete(s.externalRecords, id)
		}
		s.emit(ctx, types.EventConversationRecordsUsed{IDs: used})
		s.externalRequested = nil
		order := s.externalOrder[:0]
		for _, id := range s.externalOrder {
			if _, ok := s.externalRecords[id]; ok {
				order = append(order, id)
			}
		}
		s.externalOrder = order
	case "response.output_item.added":
		// Realtime は1回の応答で、前置き（commentary）と最終回答（final_answer）に同じ文を出すことがある。
		// 一方で、一次応答のように前置きだけを返すこともあるので、前置きはすぐには読み上げず取っておき、
		// 同じ応答に最終回答が来なかったときだけ、応答の終わり（response.done）で読み上げる。
		r := s.responses[e.ResponseID]
		if r == nil || e.Item.Type != "message" {
			return
		}
		if e.Item.Phase == "commentary" {
			s.commentary[e.Item.ID] = true
			r.commentary = append(r.commentary, &commentaryItem{id: e.Item.ID})
		} else {
			r.final = true
		}
	case "response.output_text.delta":
		r := s.responses[e.ResponseID]
		if r == nil {
			return
		}
		if s.commentary[e.ItemID] {
			if c := r.commentaryItem(e.ItemID); c != nil {
				c.text += e.Delta
			}
			return
		}
		s.outputText(ctx, r, e.ResponseID, e.ItemID, e.Delta)
	case "response.output_text.done":
		r := s.responses[e.ResponseID]
		if r == nil {
			return
		}
		if s.commentary[e.ItemID] {
			if c := r.commentaryItem(e.ItemID); c != nil && e.Text != "" {
				c.text = e.Text
			}
			return
		}
		s.outputDone(ctx, r, e.ResponseID, e.ItemID, e.Text)
	case "response.done":
		logUsage(e.Response.Usage)
		// 最終回答がなく前置きだけだった応答は、取っておいた前置きを読み上げる。
		if r := s.responses[e.Response.ID]; r != nil && !r.final {
			for _, c := range r.commentary {
				if c.text == "" {
					continue
				}
				delete(s.commentary, c.id)
				s.outputText(ctx, r, e.Response.ID, c.id, c.text)
				s.outputDone(ctx, r, e.Response.ID, c.id, c.text)
			}
		}
		if s.active == e.Response.ID {
			s.active = ""
		}
		s.maybeRespond(ctx)
	case "error":
		s.disconnect(ctx, fmt.Errorf("Realtime API: %s (%s)", e.Error.Message, e.Error.Code))
	}
}

// outputText は応答の文章の差分を読み上げへ流し、読み上げ前に止めたときの補正に使えるよう覚えておく。
func (s *stage) outputText(ctx context.Context, r *response, responseID, itemID, delta string) {
	item := s.items[itemID]
	if item == nil {
		item = &assistantItem{generation: r.generation}
		s.items[itemID] = item
		s.itemOrder = append(s.itemOrder, itemID)
	}
	s.pendingOutput[itemID] = true
	r.items[itemID] = true
	item.generated += delta
	s.emit(ctx, types.EventResponseText{ResponseText: types.ResponseText{GenerationID: r.generation, ResponseID: responseID, ItemID: itemID, Text: delta}})
}

// outputDone は応答の文章の終わりを読み上げへ知らせる。
func (s *stage) outputDone(ctx context.Context, r *response, responseID, itemID, text string) {
	if item := s.items[itemID]; item != nil && text != "" {
		item.generated = text
	}
	s.emit(ctx, types.EventResponseText{ResponseText: types.ResponseText{GenerationID: r.generation, ResponseID: responseID, ItemID: itemID, Done: true}})
}

// interruptedText は、モデルの発話がユーザーに遮られたことを履歴に残す記録の本文。
const interruptedText = "[直前の発話は途中でユーザーに遮られました]"

// recordInterruption は、世代 generation の応答に送り終えていない文章があれば、遮られたことを1件記録する。
// ユーザーが話し始めて世代が切り替わった直後に呼ぶ。ユーザーの発話の記録より前に並ぶよう、ここで記録する。
// 遮ったのはユーザー自身なので、画面には出さない。
func (s *stage) recordInterruption(ctx context.Context, generation types.GenerationID) {
	for _, id := range s.itemOrder {
		item := s.items[id]
		if item == nil || item.generation != generation || !s.pendingOutput[id] || sameSpokenText(item.generated, item.sent) {
			continue
		}
		s.saveRecord(ctx, types.ConversationRecord{ID: fmt.Sprintf("%d/%s/interrupted", s.epoch, id), Role: types.RoleSystem, Text: interruptedText, GenerationID: generation, Source: types.RealtimeSource, CreatedAt: time.Now()}, true)
		return
	}
}

// sent は読み上げの区切りを送り終えるたびに、その文章を1件の記録として足す。
// 遮られた応答は、送り終えた区切りまでが履歴に残る。区切りの ID は応答と項目の ID を含むので、接続のエポックだけを足す。
func (s *stage) sent(ctx context.Context, frame types.AudioFrame) {
	if frame.End {
		delete(s.pendingOutput, frame.ItemID)
		return
	}
	if frame.ItemID == "" {
		return
	}
	item := s.items[frame.ItemID]
	if item == nil || frame.GenerationID != item.generation {
		return
	}
	if frame.Final {
		item.sent += frame.Text
		s.saveRecord(ctx, types.ConversationRecord{ID: fmt.Sprintf("%d/%s", s.epoch, frame.SegmentID), Role: types.RoleAgent, Text: frame.Text, GenerationID: item.generation, Source: types.RealtimeSource, CreatedAt: time.Now()}, true)
	}
}

// maybeRespond は、送れる状態なら、ためている応答依頼の先頭を1件送る。
// 応答の終わり（response.done）や接続の完了など、送れない理由が解けるイベントのたびに呼ぶ。
func (s *stage) maybeRespond(ctx context.Context) {
	// ユーザーが話し足して世代が進んだら、その前のターンの依頼は送らない。ターンの外の依頼（NewGeneration）は捨てない。
	for len(s.pendingRequests) > 0 && !s.pendingRequests[0].NewGeneration && !s.cfg.Generation.IsCurrent(s.pendingRequests[0].GenerationID) {
		s.pendingRequests = s.pendingRequests[1:]
	}
	if !s.ready || len(s.pendingRequests) == 0 || s.speaking || s.awaitingInputCommit != "" || s.active != "" || len(s.repairs) > 0 {
		return
	}
	// ターンの外の発話は、読み上げ中の応答を新しい世代で止めてしまわないよう、読み上げが終わってから始める。
	if s.pendingRequests[0].NewGeneration && s.hasPendingOutput() {
		return
	}
	// 旧応答の未送信テキストを補正し、APIの反映通知が届くまで次の生成を始めない。
	repaired := false
	for _, id := range s.itemOrder {
		item := s.items[id]
		if item == nil {
			continue
		}
		if item.generation >= s.cfg.Generation.Current() || sameSpokenText(item.generated, item.sent) {
			continue
		}
		replacement := "item_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:24]
		previous := s.previous[id]
		if previous == "" {
			previous = "root"
		}
		text := item.sent
		if text == "" {
			text = "[この応答は読み上げ前に中断された]"
		} else {
			text += " [ここで発話が中断された]"
		}
		s.repairs[id] = true
		s.repairs[replacement] = true
		s.transport.send(map[string]any{"type": "conversation.item.delete", "item_id": id})
		s.transport.send(map[string]any{"type": "conversation.item.create", "previous_item_id": previous, "item": map[string]any{"id": replacement, "type": "message", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": text}}}})
		// 古いIDへの補正を重複しない。後着送信実績との微小な時間差は許容する。
		delete(s.items, id)
		s.previous[replacement] = previous
		for next, prev := range s.previous {
			if prev == id {
				s.previous[next] = replacement
			}
		}
		repaired = true
	}
	if repaired {
		return
	}
	s.flushExternal()
	s.externalRequested = make(map[string]bool, len(s.externalPending))
	for id := range s.externalPending {
		if !s.externalApplied[id] {
			continue
		}
		s.externalRequested[id] = true
	}
	req := s.pendingRequests[0]
	s.pendingRequests = s.pendingRequests[1:]
	gen := req.GenerationID
	if req.NewGeneration {
		change, _ := s.controller.Start("response/" + uuid.NewString())
		s.emit(ctx, types.EventGenerationChanged{GenerationChanged: change})
		gen = change.GenerationID
	}
	response := map[string]any{"output_modalities": []string{"text"}, "metadata": map[string]string{"generation": strconv.FormatUint(uint64(gen), 10)}}
	s.sendNotice(time.Now(), strings.TrimSpace(req.Notice))
	s.transport.send(map[string]any{"type": "response.create", "response": response})
	// response.createdを受信するまでの二重開始を防ぐ。
	s.active = "pending"
}
func encodeAudio(pcm []byte) string { return base64.StdEncoding.EncodeToString(pcm) }

// 文章分割で読み飛ばした空白は、読み上げの欠落とみなさない。
func sameSpokenText(a, b string) bool {
	normalize := func(s string) string {
		return strings.Map(func(r rune) rune {
			if unicode.IsSpace(r) {
				return -1
			}
			return r
		}, s)
	}
	return normalize(a) == normalize(b)
}
