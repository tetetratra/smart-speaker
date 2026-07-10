package timer

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Timer is an in-memory reminder-like action that should be handed back to the AI at At.
type Timer struct {
	ID        string
	At        time.Time
	Action    string
	CreatedAt time.Time
}

// Store keeps active timers in memory only.
type Store struct {
	mu     sync.RWMutex
	timers map[string]Timer
	now    func() time.Time
}

func NewStore() *Store {
	return &Store{now: time.Now}
}

func NewStoreWithClock(now func() time.Time) *Store {
	if now == nil {
		now = time.Now
	}
	return &Store{now: now}
}

func (s *Store) Create(at time.Time, action string) Timer {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timers == nil {
		s.timers = map[string]Timer{}
	}
	timer := Timer{
		ID:        uuid.NewString(),
		At:        at,
		Action:    strings.TrimSpace(action),
		CreatedAt: s.now(),
	}
	s.timers[timer.ID] = timer
	return timer
}

func (s *Store) Snapshot() []Timer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return sortedTimers(s.timers)
}

func (s *Store) Cancel(id string) (Timer, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Timer{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	timer, ok := s.timers[id]
	if ok {
		delete(s.timers, id)
	}
	return timer, ok
}

func (s *Store) PopDue(now time.Time) []Timer {
	s.mu.Lock()
	defer s.mu.Unlock()
	var due []Timer
	for id, timer := range s.timers {
		if timer.At.After(now) {
			continue
		}
		due = append(due, timer)
		delete(s.timers, id)
	}
	sort.Slice(due, func(i, j int) bool {
		if due[i].At.Equal(due[j].At) {
			return due[i].ID < due[j].ID
		}
		return due[i].At.Before(due[j].At)
	})
	return due
}

func sortedTimers(items map[string]Timer) []Timer {
	out := make([]Timer, 0, len(items))
	for _, timer := range items {
		out = append(out, timer)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].At.Equal(out[j].At) {
			return out[i].ID < out[j].ID
		}
		return out[i].At.Before(out[j].At)
	})
	return out
}
