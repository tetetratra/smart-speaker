package agentstatus

import "sync"

type Status string

const (
	StatusIdle   Status = "idle"
	StatusActive Status = "active"
)

// Store keeps the current agent status.
type Store struct {
	mu     sync.RWMutex
	status Status
}

func NewStore() *Store {
	return &Store{status: StatusIdle}
}

func (s *Store) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

func (s *Store) SetIdle() {
	s.set(StatusIdle)
}

func (s *Store) SetActive() {
	s.set(StatusActive)
}

func (s *Store) set(status Status) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = status
}
