package main

import (
	"context"

	memorystate "github.com/tetetratra/smart-speaker/internal/states/memory"
)

type manualMemoryWriter interface {
	Create(context.Context, string) (memorystate.Record, memorystate.UpsertResult, error)
	Update(context.Context, string, string) (memorystate.Record, error)
	Delete(string) error
}

type memoryTagger interface {
	CreateTags(context.Context, string) ([]string, error)
}

type memoryEmbedder interface {
	Embed(context.Context, string) ([]float64, error)
}

type manualMemoryService struct {
	store                  *memorystate.Store
	tagger                 memoryTagger
	embedder               memoryEmbedder
	duplicateMinSimilarity float64
}

func (s *manualMemoryService) Create(ctx context.Context, content string) (memorystate.Record, memorystate.UpsertResult, error) {
	tags, embedding, err := s.recalculate(ctx, content)
	if err != nil {
		return memorystate.Record{}, memorystate.UpsertResult{}, err
	}
	return s.store.Upsert(memorystate.UpsertInput{
		Content:                content,
		Tags:                   tags,
		Embedding:              embedding,
		DuplicateMinSimilarity: s.duplicateMinSimilarity,
	})
}

func (s *manualMemoryService) Update(ctx context.Context, id, content string) (memorystate.Record, error) {
	tags, embedding, err := s.recalculate(ctx, content)
	if err != nil {
		return memorystate.Record{}, err
	}
	return s.store.Update(id, memorystate.UpdateInput{
		Content:   content,
		Tags:      tags,
		Embedding: embedding,
	})
}

func (s *manualMemoryService) Delete(id string) error {
	return s.store.Delete(id)
}

func (s *manualMemoryService) recalculate(ctx context.Context, content string) ([]string, []float64, error) {
	tags, err := s.tagger.CreateTags(ctx, content)
	if err != nil {
		return nil, nil, err
	}
	searchText := memorystate.Record{Content: content, Tags: tags}.SearchText()
	embedding, err := s.embedder.Embed(ctx, searchText)
	if err != nil {
		return nil, nil, err
	}
	return tags, embedding, nil
}
