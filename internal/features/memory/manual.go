package memory

import (
	"context"
)

type manualMemoryWriter interface {
	Create(context.Context, string) (Record, UpsertResult, error)
	Update(context.Context, string, string) (Record, error)
	Delete(string) error
}

type manualMemoryService struct {
	store *Store
}

func (s *manualMemoryService) Create(_ context.Context, content string) (Record, UpsertResult, error) {
	return s.store.Upsert(UpsertInput{Content: content})
}

func (s *manualMemoryService) Update(_ context.Context, id, content string) (Record, error) {
	return s.store.Update(id, UpdateInput{Content: content})
}

func (s *manualMemoryService) Delete(id string) error {
	return s.store.Delete(id)
}
