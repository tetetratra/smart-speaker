package memory

import (
	"context"
	"errors"
	"fmt"

	memorystate "github.com/tetetratra/smart-speaker/internal/states/memory"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

type CreatorHookConfig struct {
	History                historyReader
	CandidateCreator       candidateCreator
	Embedder               embedder
	Memory                 memoryUpserter
	DuplicateMinSimilarity float64
}

type CreatorHook struct {
	history                historyReader
	candidateCreator       candidateCreator
	embedder               embedder
	memory                 memoryUpserter
	duplicateMinSimilarity float64
}

type historyReader interface {
	Snapshot() []types.ConversationRecord
}

type candidateCreator interface {
	CreateCandidates(context.Context, []types.ConversationRecord) ([]Candidate, error)
}

type embedder interface {
	Embed(context.Context, string) ([]float64, error)
}

type memoryUpserter interface {
	Upsert(memorystate.UpsertInput) (memorystate.Record, memorystate.UpsertResult, error)
}

func NewCreatorHook(cfg CreatorHookConfig) (*CreatorHook, error) {
	if cfg.History == nil {
		return nil, fmt.Errorf("memory creator hook: history is required")
	}
	if cfg.CandidateCreator == nil {
		return nil, fmt.Errorf("memory creator hook: candidate creator is required")
	}
	if cfg.Embedder == nil {
		return nil, fmt.Errorf("memory creator hook: embedder is required")
	}
	if cfg.Memory == nil {
		return nil, fmt.Errorf("memory creator hook: memory is required")
	}
	return &CreatorHook{
		history:                cfg.History,
		candidateCreator:       cfg.CandidateCreator,
		embedder:               cfg.Embedder,
		memory:                 cfg.Memory,
		duplicateMinSimilarity: cfg.DuplicateMinSimilarity,
	}, nil
}

func (h *CreatorHook) Exec(ctx context.Context) error {
	records := h.history.Snapshot()
	if len(records) == 0 {
		return nil
	}
	candidates, err := h.candidateCreator.CreateCandidates(ctx, records)
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		return nil
	}
	var errs []error
	for _, candidate := range normalizeCandidates(candidates, DefaultMaxMemoryCandidates, DefaultMaxMemoryCandidateTags) {
		searchText := memorystate.Record{Content: candidate.Content, Tags: candidate.Tags}.SearchText()
		embedding, err := h.embedder.Embed(ctx, searchText)
		if err != nil {
			errs = append(errs, fmt.Errorf("embed memory candidate %q: %w", candidate.Content, err))
			continue
		}
		_, _, err = h.memory.Upsert(memorystate.UpsertInput{
			Content:                candidate.Content,
			Tags:                   candidate.Tags,
			Embedding:              embedding,
			DuplicateMinSimilarity: h.duplicateMinSimilarity,
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("upsert memory candidate %q: %w", candidate.Content, err))
		}
	}
	return errors.Join(errs...)
}
