package conversationhistory

import (
	"sync"

	types "smart-speaker/internal/types"
)

// Store は LLM に渡す会話履歴の正本です。
type Store struct {
	mu      sync.RWMutex
	records []types.ConversationRecord
}

func NewStore() *Store {
	return &Store{}
}

func (s *Store) Append(record types.ConversationRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, cloneRecord(record))
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

func (s *Store) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = nil
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
