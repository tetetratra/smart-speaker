package agentstatus

import "testing"

func TestNewStoreStartsIdle(t *testing.T) {
	s := NewStore()
	if got := s.Status(); got != StatusIdle {
		t.Fatalf("status = %q, want %q", got, StatusIdle)
	}
}

func TestStoreTransitions(t *testing.T) {
	s := NewStore()

	s.SetActive()
	if got := s.Status(); got != StatusActive {
		t.Fatalf("status = %q, want %q", got, StatusActive)
	}

	s.SetIdle()
	if got := s.Status(); got != StatusIdle {
		t.Fatalf("status = %q, want %q", got, StatusIdle)
	}
}
