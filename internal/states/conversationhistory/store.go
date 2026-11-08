package conversationhistory

import (
	"sync"

	types "github.com/tetetratra/smart-speaker/internal/types"
)

// Store は会話履歴の正本です。realtime が記録し、再接続時の文脈の復元と会話終了時のメモリ抽出に使います。
type Store struct {
	mu      sync.RWMutex
	records []types.ConversationRecord
}

func NewStore() *Store {
	return &Store{}
}

func (s *Store) Snapshot() []types.ConversationRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]types.ConversationRecord, len(s.records))
	for i, rec := range s.records {
		out[i] = cloneRecord(rec)
	}
	return out
}

func cloneRecord(record types.ConversationRecord) types.ConversationRecord {
	if record.Metadata != nil {
		metadata := make(map[string]any, len(record.Metadata))
		for key, value := range record.Metadata {
			metadata[key] = value
		}
		record.Metadata = metadata
	}
	return record
}

// Upsert は遅れて届く文字起こしや送信実績を、元の発話位置へ反映する。
func (s *Store) Upsert(record types.ConversationRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.records {
		if s.records[i].ID == record.ID {
			s.records[i] = cloneRecord(record)
			return
		}
	}
	s.records = append(s.records, cloneRecord(record))
}

// Drain は終了した会話だけを切り離し、後から始まる会話を消さない。
func (s *Store) Drain() []types.ConversationRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.records
	s.records = nil
	return out
}
