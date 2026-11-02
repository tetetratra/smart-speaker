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

func (s *Store) Update(place string) Record {
	s.mu.Lock()
	defer s.mu.Unlock()

	record := Record{Place: place, UpdatedAt: time.Now().UTC()}
	s.current = &record
	return record
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
