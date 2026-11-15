package userlocation

import (
	"sync"
	"time"
)

// Record は直近に受け取った場所とサーバーでの更新日時を表す。
type Record struct {
	Place     string    `json:"place"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Store は現在地をメモリ上だけに保持する。ゼロ値では現在地は未設定となる。
type Store struct {
	mu      sync.RWMutex
	current *Record
}

// Update は現在地を place にし、更新後の値と更新前の値（未設定なら nil）を返す。
func (s *Store) Update(place string) (current Record, previous *Record) {
	s.mu.Lock()
	defer s.mu.Unlock()

	previous = s.current
	current = Record{Place: place, UpdatedAt: time.Now()}
	s.current = &current
	return current, previous
}

func (s *Store) Snapshot() *Record {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.current == nil {
		return nil
	}
	record := *s.current
	return &record
}
