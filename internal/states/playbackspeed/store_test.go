package playbackspeed

import "testing"

func TestNewStoreDefaultsToOne(t *testing.T) {
	s := NewStore()
	if got := s.Speed(); got != 1 {
		t.Fatalf("Speed() = %v, want 1", got)
	}
}

func TestSetSpeedAcceptsPresets(t *testing.T) {
	s := NewStore()
	for _, preset := range Presets {
		s.SetSpeed(preset)
		if got := s.Speed(); got != preset {
			t.Fatalf("Speed() = %v, want %v", got, preset)
		}
	}
}

func TestSetSpeedRejectsInvalidValues(t *testing.T) {
	s := NewStore()
	s.SetSpeed(2)
	s.SetSpeed(99)
	if got := s.Speed(); got != 1 {
		t.Fatalf("Speed() = %v, want 1", got)
	}
}

func TestNormalizeSpeed(t *testing.T) {
	if got := NormalizeSpeed(1.5); got != 1.5 {
		t.Fatalf("NormalizeSpeed(1.5) = %v, want 1.5", got)
	}
	if got := NormalizeSpeed(0); got != 1 {
		t.Fatalf("NormalizeSpeed(0) = %v, want 1", got)
	}
}
