package memory

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const currentVersion = 1

var ErrEmptyContent = errors.New("memory content is empty")
var ErrRecordNotFound = errors.New("memory record not found")

type Store struct {
	mu      sync.RWMutex
	path    string
	records []Record
}

type filePayload struct {
	Version int      `json:"version"`
	Records []Record `json:"records"`
}

func NewStore(path string) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("memory store path is empty")
	}
	records, err := loadRecords(path)
	if err != nil {
		return nil, err
	}
	return &Store{path: path, records: records}, nil
}

func (s *Store) Snapshot() []Record {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneRecords(s.records)
}

func (s *Store) Upsert(input UpsertInput) (Record, UpsertResult, error) {
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return Record{}, UpsertResult{}, ErrEmptyContent
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	// 言い換えの重複は抽出時に LLM が既存メモリを見て防ぐので、ここでは同じ本文だけを弾く。
	if idx, ok := s.findSameContentLocked(content); ok {
		return s.records[idx], UpsertResult{Created: false}, nil
	}

	record := Record{
		ID:        newRecordID(now, len(s.records)+1),
		Content:   content,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.records = append(s.records, record)
	if err := s.saveLocked(); err != nil {
		return Record{}, UpsertResult{}, err
	}
	return record, UpsertResult{Created: true}, nil
}

func (s *Store) Update(id string, input UpdateInput) (Record, error) {
	id = strings.TrimSpace(id)
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return Record{}, ErrEmptyContent
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.records {
		if s.records[i].ID != id {
			continue
		}
		s.records[i].Content = content
		s.records[i].UpdatedAt = time.Now().UTC()
		if err := s.saveLocked(); err != nil {
			return Record{}, err
		}
		return s.records[i], nil
	}
	return Record{}, ErrRecordNotFound
}

func (s *Store) Delete(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return ErrRecordNotFound
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.records {
		if s.records[i].ID != id {
			continue
		}
		s.records = append(s.records[:i], s.records[i+1:]...)
		return s.saveLocked()
	}
	return ErrRecordNotFound
}

func (s *Store) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = nil
	return s.saveLocked()
}

func (s *Store) findSameContentLocked(content string) (int, bool) {
	for i, record := range s.records {
		if SameContent(record.Content, content) {
			return i, true
		}
	}
	return 0, false
}

// SameContent は前後の空白と英字の大文字小文字を無視して本文が同じかを判定する。
func SameContent(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func (s *Store) saveLocked() error {
	return saveRecords(s.path, s.records)
}

func loadRecords(path string) ([]Record, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var payload filePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	if payload.Version != currentVersion {
		return nil, fmt.Errorf("unsupported memory store version: %d", payload.Version)
	}
	records := cloneRecords(payload.Records)
	for i := range records {
		records[i].Content = strings.TrimSpace(records[i].Content)
	}
	return records, nil
}

func saveRecords(path string, records []Record) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(filePayload{
		Version: currentVersion,
		Records: cloneRecords(records),
	}, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, raw, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func cloneRecords(records []Record) []Record {
	if records == nil {
		return nil
	}
	cloned := make([]Record, len(records))
	copy(cloned, records)
	return cloned
}

func newRecordID(now time.Time, seq int) string {
	return fmt.Sprintf("%d-%d", now.UnixNano(), seq)
}
