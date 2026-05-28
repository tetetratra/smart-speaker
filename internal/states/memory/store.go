package memory

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
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
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return Record{}, UpsertResult{}, ErrEmptyContent
	}
	tags := normalizeTags(input.Tags)
	embedding := cloneFloat64s(input.Embedding)

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	if idx, result, ok := s.findDuplicateLocked(DuplicateInput{
		Content:       content,
		Tags:          tags,
		Embedding:     embedding,
		MinSimilarity: input.DuplicateMinSimilarity,
	}); ok {
		updated := s.records[idx]
		updated.Content = content
		updated.Tags = tags
		updated.Embedding = embedding
		updated.UpdatedAt = now
		s.records[idx] = updated
		if err := s.saveLocked(); err != nil {
			return Record{}, UpsertResult{}, err
		}
		result.Created = false
		return cloneRecord(updated), result, nil
	}

	record := Record{
		ID:        newRecordID(now, len(s.records)+1),
		Content:   content,
		Tags:      tags,
		Embedding: embedding,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.records = append(s.records, record)
	if err := s.saveLocked(); err != nil {
		return Record{}, UpsertResult{}, err
	}
	return cloneRecord(record), UpsertResult{Created: true}, nil
}

func (s *Store) FindDuplicate(input DuplicateInput) (Record, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	idx, _, ok := s.findDuplicateLocked(normalizeDuplicateInput(input))
	if !ok {
		return Record{}, false
	}
	return cloneRecord(s.records[idx]), true
}

func (s *Store) Search(query []float64, opts SearchOptions) []SearchResult {
	s.mu.RLock()
	defer s.mu.RUnlock()

	results := make([]SearchResult, 0, len(s.records))
	for _, record := range s.records {
		similarity, ok := cosineSimilarity(query, record.Embedding)
		if !ok || similarity < opts.MinSimilarity {
			continue
		}
		results = append(results, SearchResult{
			Record:     cloneRecord(record),
			Similarity: similarity,
		})
	}
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].Similarity > results[j].Similarity
	})
	if opts.Limit > 0 && len(results) > opts.Limit {
		results = results[:opts.Limit]
	}
	return results
}

func (s *Store) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = nil
	return s.saveLocked()
}

func (s *Store) findDuplicateLocked(input DuplicateInput) (int, UpsertResult, bool) {
	normalized := normalizeDuplicateInput(input)
	for i, record := range s.records {
		if strings.EqualFold(strings.TrimSpace(record.Content), normalized.Content) {
			return i, UpsertResult{DuplicateReason: "content"}, true
		}
	}
	if len(normalized.Tags) > 0 {
		for i, record := range s.records {
			if tagsEqual(normalizeTags(record.Tags), normalized.Tags) {
				return i, UpsertResult{DuplicateReason: "tags"}, true
			}
		}
	}
	if normalized.MinSimilarity > 0 {
		for i, record := range s.records {
			similarity, ok := cosineSimilarity(normalized.Embedding, record.Embedding)
			if ok && similarity >= normalized.MinSimilarity {
				return i, UpsertResult{DuplicateReason: "embedding", Similarity: similarity}, true
			}
		}
	}
	return 0, UpsertResult{}, false
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

func normalizeDuplicateInput(input DuplicateInput) DuplicateInput {
	input.Content = strings.TrimSpace(input.Content)
	input.Tags = normalizeTags(input.Tags)
	input.Embedding = cloneFloat64s(input.Embedding)
	return input
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
	sort.Slice(normalized, func(i, j int) bool {
		return strings.ToLower(normalized[i]) < strings.ToLower(normalized[j])
	})
	return normalized
}

func tagsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !strings.EqualFold(a[i], b[i]) {
			return false
		}
	}
	return true
}

func newRecordID(now time.Time, seq int) string {
	return fmt.Sprintf("%d-%d", now.UnixNano(), seq)
}
