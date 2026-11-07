package realtime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/tetetratra/smart-speaker/internal/conversation"
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
		CallID               string `json:"call_id"`
		Name, Arguments      string
	} `json:"item"`
	Response struct {
		ID, Status string
		Metadata   map[string]string
		Output     []struct{ ID, Type, Text string }
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
		for _, result := range s.contextResults {
			s.sendResultContext(result)
		}
		s.contextResults = nil
		pending := s.pendingText
		s.pendingText = nil
		for _, text := range pending {
			s.handle(ctx, types.Event{Payload: text})
		}
		s.maybeRespond(ctx)
	case "input_audio_buffer.speech_started":
		// 呼びかけ後の会話中は、ユーザーの発話ごとに会話中の時間を数え直す。
		if s.cfg.WakeMode != nil {
			s.cfg.WakeMode.Touch(time.Now())
		}
		s.awaitingInputCommit = e.ItemID
		s.silentGeneration = 0
		s.speaking = true
		s.lastActivity = time.Now()
		s.wantResponse = false
		change, ok := s.controller.Start(fmt.Sprintf("%d/%s", s.epoch, e.ItemID))
		if !ok {
			return
		}
		change.ItemID = e.ItemID
		s.userGenerations[e.ItemID] = change.GenerationID
		s.cfg.History.Upsert(types.ConversationRecord{ID: fmt.Sprintf("%d/%s", s.epoch, e.ItemID), Role: types.RoleUser, GenerationID: change.GenerationID, CreatedAt: time.Now(), Source: "audio", Text: "[文字起こし未完了の音声発話]"})
		s.emit(ctx, types.Event{Kind: types.EventGenerationChanged, Payload: change})
		s.emit(ctx, types.Event{Kind: types.EventSpeechStart, Payload: types.SpeechEvent{Source: "realtime-vad", CapturedAt: time.Now()}})
	case "input_audio_buffer.speech_stopped":
		s.speaking = false
		s.lastActivity = time.Now()
		s.emit(ctx, types.Event{Kind: types.EventSpeechEnd, Payload: types.SpeechEvent{Source: "realtime-vad", CapturedAt: time.Now()}})
	case "input_audio_buffer.committed":
		if s.awaitingInputCommit == e.ItemID {
			s.awaitingInputCommit = ""
		}
		// 発話終了だけでなく、APIが入力itemを確定した後に応答を要求する。
		s.wantResponse = true
		s.maybeRespond(ctx)
	case "conversation.item.input_audio_transcription.completed":
		gen, ok := s.userGenerations[e.ItemID]
		if !ok {
			return
		}
		s.record(ctx, e.ItemID, types.RoleUser, e.Transcript, gen, "realtime-transcription")
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
		s.responses[e.Response.ID] = &response{silent: s.silentGeneration != 0 && s.silentGeneration == types.GenerationID(gen64), generation: types.GenerationID(gen64), items: map[string]bool{}}
		s.active = e.Response.ID
	case "response.output_text.delta":
		r := s.responses[e.ResponseID]
		if r == nil {
			return
		}
		item := s.items[e.ItemID]
		if item == nil {
			item = &assistantItem{generation: r.generation, silent: r.silent}
			s.items[e.ItemID] = item
			s.itemOrder = append(s.itemOrder, e.ItemID)
		}
		if !r.silent {
			s.pendingOutput[e.ItemID] = true
		}
		r.items[e.ItemID] = true
		item.generated += e.Delta
		s.saveAssistant(e.ItemID, item)
		if !r.silent {
			s.emit(ctx, types.Event{Kind: types.EventResponseText, Payload: types.ResponseText{GenerationID: r.generation, ResponseID: e.ResponseID, ItemID: e.ItemID, Text: e.Delta}})
		}
	case "response.output_text.done":
		r := s.responses[e.ResponseID]
		if r == nil {
			return
		}
		item := s.items[e.ItemID]
		if item != nil {
			if e.Text != "" {
				item.generated = e.Text
				s.saveAssistant(e.ItemID, item)
			}
			s.emit(ctx, types.Event{Kind: types.EventRealtimeOutput, Payload: types.OutputLine{Role: types.RoleAgent, Text: item.generated, GenerationID: r.generation, ResponseID: e.ResponseID, Final: true}})
		}
		if !r.silent {
			s.emit(ctx, types.Event{Kind: types.EventResponseText, Payload: types.ResponseText{GenerationID: r.generation, ResponseID: e.ResponseID, ItemID: e.ItemID, Done: true}})
		}
	case "response.function_call_arguments.done":
		r := s.responses[e.ResponseID]
		if r == nil {
			return
		}
		if _, exists := s.calls[e.CallID]; exists {
			return
		}
		if !s.cfg.Generation.IsCurrent(r.generation) {
			return
		}
		s.calls[e.CallID] = &call{epoch: s.epoch, generation: r.generation, silent: r.silent}
		req := types.ToolRequest{ResponseID: e.ResponseID, ToolCallID: e.CallID, Name: e.Name, Arguments: json.RawMessage(e.Arguments), GenerationID: r.generation}
		s.record(ctx, "call/"+e.CallID, types.RoleToolCall, e.Name+": "+e.Arguments, r.generation, e.Name)
		if e.Name == waitToolName {
			s.waitForUser(ctx, req)
			return
		}
		s.emit(ctx, types.Event{Kind: types.EventToolRequest, Payload: req})
	case "response.done":
		if s.active == e.Response.ID {
			s.active = ""
		}
		s.maybeRespond(ctx)
	case "error":
		s.disconnect(ctx, fmt.Errorf("Realtime API: %s (%s)", e.Error.Message, e.Error.Code))
	}
}
func (s *stage) saveAssistant(itemID string, item *assistantItem) {
	if item.silent {
		s.cfg.History.Upsert(types.ConversationRecord{ID: fmt.Sprintf("%d/%s", s.epoch, itemID), Role: types.RoleAgent, Text: item.generated, GenerationID: item.generation, Source: "automation", CreatedAt: time.Now(), Metadata: map[string]any{"silent": true}})
		return
	}
	s.cfg.History.Upsert(types.ConversationRecord{ID: fmt.Sprintf("%d/%s", s.epoch, itemID), Role: types.RoleAgent, Text: item.sent, GenerationID: item.generation, Source: "realtime", CreatedAt: time.Now(), Metadata: map[string]any{"generated_text": item.generated, "interrupted": !sameSpokenText(item.sent, item.generated)}})
}
func (s *stage) sent(frame types.AudioFrame) {
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
		s.saveAssistant(frame.ItemID, item)
	}
}
func (s *stage) maybeRespond(ctx context.Context) {
	s.discardOldSilentToolResponses()
	if !s.ready || (!s.wantResponse && len(s.pendingToolResponses) == 0) || s.speaking || s.awaitingInputCommit != "" || s.active != "" || len(s.repairs) > 0 {
		return
	}
	// 旧応答の未送信テキストを補正し、APIの反映通知が届くまで次の生成を始めない。
	repaired := false
	for _, id := range s.itemOrder {
		item := s.items[id]
		if item == nil {
			continue
		}
		if item.silent || item.generation >= s.cfg.Generation.Current() || sameSpokenText(item.generated, item.sent) {
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
	for _, c := range s.calls {
		if c.epoch == s.epoch && c.generation == s.cfg.Generation.Current() && !c.finished {
			return
		}
	}
	s.wantResponse = false
	response := map[string]any{"output_modalities": []string{"text"}, "metadata": map[string]string{"generation": strconv.FormatUint(uint64(s.cfg.Generation.Current()), 10)}}
	// 接続を維持したまま日付が変わっても、復元文脈と指示を保って日時を更新する。
	response["instructions"] = conversation.WithCurrentTime(s.toolResultInstructions(), time.Now())
	s.transport.send(map[string]any{"type": "response.create", "response": response})
	// 送信まで保持する一回用の指示。報告完了を管理するキューではない。
	s.pendingToolResponses = nil
	// response.createdを受信するまでの二重開始を防ぐ。
	s.active = "pending"
}
func (s *stage) toolResult(ctx context.Context, result types.ToolResultRecord) {
	c := s.calls[result.ToolCallID]
	if c == nil || c.finished {
		return
	}
	c.finished = true
	s.cfg.History.Upsert(types.ConversationRecord{ID: fmt.Sprintf("%d/result/%s", c.epoch, result.ToolCallID), Role: types.RoleToolResult, Text: string(result.Output), GenerationID: c.generation, Source: result.Name, CreatedAt: time.Now()})
	s.emit(ctx, types.Event{Kind: types.EventRealtimeOutput, Payload: types.OutputLine{Role: types.RoleToolResult, Text: string(result.Output), Source: result.Name, GenerationID: c.generation, Final: true}})
	if c.epoch != s.epoch || s.transport == nil {
		if s.ready && s.transport != nil {
			s.sendResultContext(result)
		} else {
			s.contextResults = append(s.contextResults, result)
		}
	} else {
		s.transport.send(map[string]any{"type": "conversation.item.create", "item": map[string]any{"type": "function_call_output", "call_id": result.ToolCallID, "output": string(result.Output)}})
	}
	// 無音ジョブの旧結果を、新しい通常会話へ読み上げとして持ち込まない。
	if (!c.silent || s.cfg.Generation.IsCurrent(c.generation)) && s.respondAfterTool(result) {
		s.pendingToolResponses = append(s.pendingToolResponses, toolCompletion{CallID: result.ToolCallID, Name: result.Name, generation: c.generation, silent: c.silent})
	}
	s.maybeRespond(ctx)
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

// 再接続前のcall_idを新接続へ再送せず、確定した操作結果だけを文脈に追加する。
func (s *stage) sendResultContext(result types.ToolResultRecord) {
	text := "過去の操作の実行結果（操作を再実行しない）: call_id=" + result.ToolCallID + " name=" + result.Name + " " + string(result.Output)
	s.transport.send(map[string]any{"type": "conversation.item.create", "item": map[string]any{"type": "message", "role": "user", "content": []any{map[string]any{"type": "input_text", "text": text}}}})
}
