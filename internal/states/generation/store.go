package generation

import (
	"sync"

	types "github.com/tetetratra/smart-speaker/internal/types"
)

// Store は現在の会話世代idの正本です。
type Store struct {
	mu      sync.RWMutex
	current types.GenerationID
}

func NewStore() *Store {
	return &Store{}
}

func (s *Store) Next() types.GenerationID {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current++
	return s.current
}

func (s *Store) Current() types.GenerationID {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

func (s *Store) IsCurrent(id types.GenerationID) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return id == s.current
}

func (s *Store) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current = 0
}
