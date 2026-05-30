package playbackspeed

import (
	"math"
	"sync"
)

// Preset speeds supported by the UI and server store.
var Presets = []float64{1, 1.5, 2, 3}

const defaultSpeed = 1

// Store keeps the current agent speech playback speed multiplier.
type Store struct {
	mu    sync.RWMutex
	speed float64
}

func NewStore() *Store {
	return &Store{speed: defaultSpeed}
}

func (s *Store) Speed() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.speed
}

func (s *Store) SetSpeed(speed float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.speed = NormalizeSpeed(speed)
}

// NormalizeSpeed maps arbitrary input to a supported preset, defaulting to 1.
func NormalizeSpeed(speed float64) float64 {
	for _, preset := range Presets {
		if math.Abs(speed-preset) < 1e-9 {
			return preset
		}
	}
	return defaultSpeed
}
