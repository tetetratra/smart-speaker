package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	memorystate "github.com/tetetratra/smart-speaker/internal/states/memory"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

const (
	DefaultContextQueryRecordLimit = 8
	DefaultContextSearchLimit      = 3
	DefaultContextMinSimilarity    = 0.7
)

type ContextProviderConfig struct {
	Embedder         contextEmbedder
	Memory           memorySearcher
	QueryRecordLimit int
	SearchLimit      int
	MinSimilarity    float64
}

type ContextProvider struct {
	embedder         contextEmbedder
	memory           memorySearcher
	queryRecordLimit int
	searchLimit      int
	minSimilarity    float64
}

type contextEmbedder interface {
	Embed(context.Context, string) ([]float64, error)
}

type memorySearcher interface {
	Search([]float64, memorystate.SearchOptions) []memorystate.SearchResult
}

func NewContextProvider(cfg ContextProviderConfig) (*ContextProvider, error) {
	if cfg.Embedder == nil {
		return nil, fmt.Errorf("memory context provider: embedder is required")
	}
	if cfg.Memory == nil {
		return nil, fmt.Errorf("memory context provider: memory is required")
	}
	queryRecordLimit := cfg.QueryRecordLimit
	if queryRecordLimit <= 0 {
		queryRecordLimit = DefaultContextQueryRecordLimit
	}
	searchLimit := cfg.SearchLimit
	if searchLimit <= 0 {
		searchLimit = DefaultContextSearchLimit
	}
	minSimilarity := cfg.MinSimilarity
	if minSimilarity <= 0 {
		minSimilarity = DefaultContextMinSimilarity
	}
	if minSimilarity > 1 {
		return nil, fmt.Errorf("memory context provider: min similarity must be <= 1")
	}
	return &ContextProvider{
		embedder:         cfg.Embedder,
		memory:           cfg.Memory,
		queryRecordLimit: queryRecordLimit,
		searchLimit:      searchLimit,
		minSimilarity:    minSimilarity,
	}, nil
}

func (p *ContextProvider) BuildContext(ctx context.Context, records []types.ConversationRecord) ([]types.ChatMessage, error) {
	query := p.buildQuery(records)
	if query == "" {
		return nil, nil
	}
	embedding, err := p.embedder.Embed(ctx, query)
	if err != nil {
		return nil, err
	}
	results := p.memory.Search(embedding, memorystate.SearchOptions{
		MinSimilarity: p.minSimilarity,
		Limit:         p.searchLimit,
	})
	return memoryContextMessages(results)
}

func (p *ContextProvider) buildQuery(records []types.ConversationRecord) string {
	eligible := make([]types.ConversationRecord, 0, len(records))
	for _, record := range records {
		if !isMemoryQueryRole(record.Role) || strings.TrimSpace(record.Text) == "" {
			continue
		}
		eligible = append(eligible, record)
	}
	if len(eligible) == 0 {
		return ""
	}
	if len(eligible) > p.queryRecordLimit {
		eligible = eligible[len(eligible)-p.queryRecordLimit:]
	}
	lines := make([]string, 0, len(eligible))
	for _, record := range eligible {
		lines = append(lines, strings.TrimSpace(record.Role)+": "+strings.TrimSpace(record.Text))
	}
	return strings.Join(lines, "\n")
}

func isMemoryQueryRole(role string) bool {
	switch strings.TrimSpace(role) {
	case types.RoleUser, types.RoleAgent, types.RoleSystem:
		return true
	default:
		return false
	}
}

func memoryContextMessages(results []memorystate.SearchResult) ([]types.ChatMessage, error) {
	memories := make([]memoryContextItem, 0, len(results))
	for _, result := range results {
		content := strings.TrimSpace(result.Record.Content)
		if content == "" {
			continue
		}
		memories = append(memories, memoryContextItem{Content: content})
	}
	if len(memories) == 0 {
		return nil, nil
	}
	payload := memoryContextPayload{
		Type:     "memory_context",
		Memories: memories,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return []types.ChatMessage{{Role: types.RoleSystem, Content: string(encoded)}}, nil
}

type memoryContextPayload struct {
	Type     string              `json:"type"`
	Memories []memoryContextItem `json:"memories"`
}

type memoryContextItem struct {
	Content string `json:"content"`
}
