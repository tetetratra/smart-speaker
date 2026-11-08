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

type manualMemoryService struct {
	store *memorystate.Store
}

func (s *manualMemoryService) Create(_ context.Context, content string) (memorystate.Record, memorystate.UpsertResult, error) {
	return s.store.Upsert(memorystate.UpsertInput{Content: content})
}

func (s *manualMemoryService) Update(_ context.Context, id, content string) (memorystate.Record, error) {
	return s.store.Update(id, memorystate.UpdateInput{Content: content})
}

func (s *manualMemoryService) Delete(id string) error {
	return s.store.Delete(id)
}
