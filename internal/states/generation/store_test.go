package generation

import "testing"

func TestStoreNextCurrentAndReset(t *testing.T) {
	store := NewStore()
	if got := store.Current(); got != 0 {
		t.Fatalf("Current() = %d, want 0", got)
	}
	if got := store.Next(); got != 1 {
		t.Fatalf("first Next() = %d, want 1", got)
	}
	if got := store.Next(); got != 2 {
		t.Fatalf("second Next() = %d, want 2", got)
	}
	if !store.IsCurrent(2) {
		t.Fatal("IsCurrent(2) = false, want true")
	}
	if store.IsCurrent(1) {
		t.Fatal("IsCurrent(1) = true, want false")
	}
	store.Reset()
	if got := store.Current(); got != 0 {
		t.Fatalf("Current() after Reset() = %d, want 0", got)
	}
}
