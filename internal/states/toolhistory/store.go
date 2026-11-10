package toolhistory

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/tetetratra/smart-speaker/internal/types"
)

type Entry struct {
	ID           string             `json:"id"`
	CallID       string             `json:"call_id"`
	Name         string             `json:"name"`
	GenerationID types.GenerationID `json:"generation_id"`
	Arguments    string             `json:"arguments"`
	Output       string             `json:"output"`
	Status       string             `json:"status"`
	StartedAt    time.Time          `json:"started_at"`
	FinishedAt   time.Time          `json:"finished_at,omitzero"`
}

// Store は最新の呼び出しを保持する。画面の再読み込みでは消去しない。
type Store struct {
	mu       sync.RWMutex
	limit    int
	entries  []Entry
	watchers map[chan struct{}]struct{}
}

func NewStore(limit int) *Store {
	if limit < 1 {
		limit = 200
	}
	return &Store{limit: limit}
}

func (s *Store) Begin(req types.ToolRequest) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := uuid.NewString()
	s.entries = append(s.entries, Entry{ID: id, CallID: req.ToolCallID, Name: req.Name, GenerationID: req.GenerationID, Arguments: string(req.Arguments), Status: "running", StartedAt: time.Now()})
	if len(s.entries) > s.limit {
		s.entries = append([]Entry(nil), s.entries[len(s.entries)-s.limit:]...)
	}
	s.notifyLocked()
	return id
}

func (s *Store) Finish(id string, result types.ToolResultRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.entries {
		if s.entries[i].ID != id {
			continue
		}
		entry := &s.entries[i]
		entry.Output = string(result.Output)
		entry.Status = "succeeded"
		var payload map[string]any
		if err := json.Unmarshal(result.Output, &payload); err != nil || payload["error"] != nil {
			entry.Status = "failed"
		}
		entry.FinishedAt = time.Now()
		s.notifyLocked()
		return
	}
}

func (s *Store) Snapshot() []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries := make([]Entry, len(s.entries))
	for i, entry := range s.entries {
		entries[len(entries)-1-i] = entry
	}
	return entries
}

// Subscribe は接続時と履歴更新時に通知する。通知が溜まった場合はまとめ、最新の Snapshot を送れるようにする。
func (s *Store) Subscribe() (<-chan struct{}, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.watchers == nil {
		s.watchers = make(map[chan struct{}]struct{})
	}
	changed := make(chan struct{}, 1)
	s.watchers[changed] = struct{}{}
	changed <- struct{}{}
	return changed, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.watchers, changed)
	}
}

func (s *Store) notifyLocked() {
	for changed := range s.watchers {
		select {
		case changed <- struct{}{}:
		default:
		}
	}
}
