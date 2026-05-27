package memory

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const currentVersion = 1

var ErrEmptyContent = errors.New("memory content is empty")

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
	return Record{}, UpsertResult{}, fmt.Errorf("memory upsert is not implemented")
}

func (s *Store) FindDuplicate(input DuplicateInput) (Record, bool) {
	return Record{}, false
}

func (s *Store) Search(query []float64, opts SearchOptions) []SearchResult {
	return nil
}

func (s *Store) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = nil
	return s.saveLocked()
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
		records[i].Tags = normalizeTags(records[i].Tags)
		records[i].Embedding = cloneFloat64s(records[i].Embedding)
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
	for i, record := range records {
		cloned[i] = cloneRecord(record)
	}
	return cloned
}

func normalizeTags(tags []string) []string {
	seen := map[string]struct{}{}
	normalized := make([]string, 0, len(tags))
	for _, tag := range tags {
		value := strings.TrimSpace(tag)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, value)
	}
	return normalized
}
