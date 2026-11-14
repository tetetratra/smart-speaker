package conversationhistory

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	types "github.com/tetetratra/smart-speaker/internal/types"
)

const currentVersion = 1

// Store は会話履歴の正本です。会話管理が記録し、再接続時の文脈の復元と古い履歴のメモリ化に使います。
// 複数日の会話を再起動後も引き継ぐため、変更のたびにファイルへ保存します。
type Store struct {
	mu      sync.RWMutex
	path    string
	records []types.ConversationRecord
}

type filePayload struct {
	Version int                        `json:"version"`
	Records []types.ConversationRecord `json:"records"`
}

// NewStore は path の履歴を読み込む。ファイルがなければ空の履歴から始める。
func NewStore(path string) (*Store, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Store{path: path}, nil
	}
	if err != nil {
		return nil, err
	}
	var payload filePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("会話履歴の読み込み %s: %w", path, err)
	}
	if payload.Version != currentVersion {
		return nil, fmt.Errorf("会話履歴の形式に未対応です: version %d", payload.Version)
	}
	return &Store{path: path, records: payload.Records}, nil
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
func (s *Store) Upsert(record types.ConversationRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.records {
		if s.records[i].ID == record.ID {
			s.records[i] = cloneRecord(record)
			return s.saveLocked()
		}
	}
	s.records = append(s.records, cloneRecord(record))
	return s.saveLocked()
}

// Remove はメモリ化を終えた記録だけを消す。その間に追加・更新された別の記録は残す。
func (s *Store) Remove(ids []string) error {
	remove := make(map[string]bool, len(ids))
	for _, id := range ids {
		remove[id] = true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	retained := s.records[:0]
	for _, record := range s.records {
		if !remove[record.ID] {
			retained = append(retained, record)
		}
	}
	s.records = retained
	return s.saveLocked()
}

// saveLocked はメモリのストアと同じく、一時ファイルを書いてから置き換える。
func (s *Store) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	raw, err := json.Marshal(filePayload{Version: currentVersion, Records: s.records})
	if err != nil {
		return err
	}
	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, raw, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}
