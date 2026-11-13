package realtime

import (
	"encoding/json"
	"github.com/tetetratra/smart-speaker/internal/types"
)

// addContext は情報追加だけでは接続・応答・割り込みを開始しない。
func (s *stage) addContext(input types.ConversationInput) {
	if s.externalRecords == nil {
		s.externalRecords = map[string]types.ConversationRecord{}
	}
	if s.externalPending == nil {
		s.externalPending = map[string]bool{}
	}
	if _, exists := s.externalRecords[input.Record.ID]; !exists {
		s.externalOrder = append(s.externalOrder, input.Record.ID)
	}
	s.externalRecords[input.Record.ID] = input.Record
	s.externalPending[input.Record.ID] = true
	if s.ready && s.transport != nil {
		s.sendContext(input.Record)
	}
}

func (s *stage) sendContext(record types.ConversationRecord) {
	if s.externalApplied == nil {
		s.externalApplied = map[string]bool{}
	}
	if s.externalApplied[record.ID] {
		return
	}
	text := record.Text
	// 情報追加の指示を、次の応答への新しい操作依頼と混同させない。
	if record.Metadata != nil {
		// メタデータは実行命令として扱わず、本文とともにデータとして渡す。
		raw, _ := json.Marshal(record.Metadata)
		if len(raw) > 2 {
			text += "\n付随情報: " + string(raw)
		}
	}
	s.transport.send(map[string]any{"type": "conversation.item.create", "item": map[string]any{"type": "message", "role": "user", "content": []any{map[string]any{"type": "input_text", "text": text}}}})
	s.externalApplied[record.ID] = true
}

// flushExternal は接続復元中に届いた記録を、応答開始より先に反映する。
func (s *stage) flushExternal() {
	for _, id := range s.externalOrder {
		if record, ok := s.externalRecords[id]; ok {
			s.sendContext(record)
		}
	}
}
