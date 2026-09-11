package generation

import (
	"sync"

	types "github.com/tetetratra/smart-speaker/internal/types"
)

type EventDisposition int

const (
	EventDispositionDrop EventDisposition = iota
	EventDispositionAllow
	EventDispositionHold
)

type pendingInterruption struct {
	pausedID    types.GenerationID
	candidateID types.GenerationID
}

// Store は現在の会話世代idの正本です。
type Store struct {
	mu          sync.RWMutex
	current     types.GenerationID
	pending     *pendingInterruption
	subscribers map[chan struct{}]struct{}
}

func NewStore() *Store {
	return &Store{}
}

func (s *Store) Next() types.GenerationID {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current++
	s.pending = nil
	s.notifyLocked()
	return s.current
}

func (s *Store) BeginInterruption() types.GenerationID {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending != nil {
		return s.pending.candidateID
	}
	pausedID := s.current
	s.current++
	s.pending = &pendingInterruption{
		pausedID:    pausedID,
		candidateID: s.current,
	}
	s.notifyLocked()
	return s.current
}

func (s *Store) Current() types.GenerationID {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

func (s *Store) Pending() (pausedID, candidateID types.GenerationID, ok bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.pending == nil {
		return 0, 0, false
	}
	return s.pending.pausedID, s.pending.candidateID, true
}

func (s *Store) IsCurrent(id types.GenerationID) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return id == s.current
}

func (s *Store) Disposition(id types.GenerationID) EventDisposition {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.pending != nil {
		switch id {
		case s.pending.candidateID:
			return EventDispositionAllow
		case s.pending.pausedID:
			return EventDispositionHold
		default:
			return EventDispositionDrop
		}
	}
	if id == s.current {
		return EventDispositionAllow
	}
	return EventDispositionDrop
}

func (s *Store) ResumeIfPending(candidateID types.GenerationID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending == nil || s.pending.candidateID != candidateID {
		return false
	}
	s.current = s.pending.pausedID
	s.pending = nil
	s.notifyLocked()
	return true
}

func (s *Store) ConfirmIfPending(candidateID types.GenerationID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending == nil || s.pending.candidateID != candidateID {
		return false
	}
	s.current = s.pending.candidateID
	s.pending = nil
	s.notifyLocked()
	return true
}

func (s *Store) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current = 0
	s.pending = nil
	s.notifyLocked()
}

func (s *Store) Subscribe() (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	s.mu.Lock()
	if s.subscribers == nil {
		s.subscribers = map[chan struct{}]struct{}{}
	}
	s.subscribers[ch] = struct{}{}
	s.mu.Unlock()
	unsubscribe := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if _, ok := s.subscribers[ch]; !ok {
			return
		}
		delete(s.subscribers, ch)
		close(ch)
	}
	return ch, unsubscribe
}

func (s *Store) notifyLocked() {
	for ch := range s.subscribers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
