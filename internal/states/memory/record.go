package memory

import (
	"strings"
	"time"
)

// Record is a persisted memory item.
type Record struct {
	ID        string    `json:"id"`
	Content   string    `json:"content"`
	Tags      []string  `json:"tags,omitempty"`
	Embedding []float64 `json:"embedding,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type UpsertInput struct {
	Content                string
	Tags                   []string
	Embedding              []float64
	DuplicateMinSimilarity float64
}

type DuplicateInput struct {
	Content       string
	Tags          []string
	Embedding     []float64
	MinSimilarity float64
}

type UpsertResult struct {
	Created         bool
	DuplicateReason string
	Similarity      float64
}

type SearchOptions struct {
	MinSimilarity float64
	Limit         int
}

type SearchResult struct {
	Record     Record
	Similarity float64
}

func (r Record) SearchText() string {
	parts := make([]string, 0, 1+len(r.Tags))
	if content := strings.TrimSpace(r.Content); content != "" {
		parts = append(parts, content)
	}
	for _, tag := range r.Tags {
		if normalized := strings.TrimSpace(tag); normalized != "" {
			parts = append(parts, normalized)
		}
	}
	return strings.Join(parts, " ")
}

func cloneRecord(record Record) Record {
	record.Tags = cloneStrings(record.Tags)
	record.Embedding = cloneFloat64s(record.Embedding)
	return record
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}

func cloneFloat64s(values []float64) []float64 {
	if values == nil {
		return nil
	}
	cloned := make([]float64, len(values))
	copy(cloned, values)
	return cloned
}
