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

func TestStoreBeginInterruptionAndResume(t *testing.T) {
	store := NewStore()
	store.Next()

	if got := store.BeginInterruption(); got != 2 {
		t.Fatalf("BeginInterruption() = %d, want 2", got)
	}
	paused, candidate, ok := store.Pending()
	if !ok {
		t.Fatal("Pending() ok = false, want true")
	}
	if paused != 1 || candidate != 2 {
		t.Fatalf("Pending() = (%d, %d), want (1, 2)", paused, candidate)
	}
	if got := store.Disposition(1); got != EventDispositionHold {
		t.Fatalf("Disposition(paused) = %d, want hold", got)
	}
	if got := store.Disposition(2); got != EventDispositionAllow {
		t.Fatalf("Disposition(candidate) = %d, want allow", got)
	}

	if !store.ResumeIfPending(2) {
		t.Fatal("ResumeIfPending(2) = false, want true")
	}
	if got := store.Current(); got != 1 {
		t.Fatalf("Current() after resume = %d, want 1", got)
	}
	if _, _, ok := store.Pending(); ok {
		t.Fatal("Pending() after resume ok = true, want false")
	}
	if got := store.Disposition(1); got != EventDispositionAllow {
		t.Fatalf("Disposition(resumed) = %d, want allow", got)
	}
	if got := store.Disposition(2); got != EventDispositionDrop {
		t.Fatalf("Disposition(candidate after resume) = %d, want drop", got)
	}
}

func TestStoreBeginInterruptionAndConfirm(t *testing.T) {
	store := NewStore()
	store.Next()
	if got := store.BeginInterruption(); got != 2 {
		t.Fatalf("BeginInterruption() = %d, want 2", got)
	}
	if got := store.BeginInterruption(); got != 2 {
		t.Fatalf("second BeginInterruption() = %d, want existing candidate 2", got)
	}

	if !store.ConfirmIfPending(2) {
		t.Fatal("ConfirmIfPending(2) = false, want true")
	}
	if got := store.Current(); got != 2 {
		t.Fatalf("Current() after confirm = %d, want 2", got)
	}
	if got := store.Disposition(1); got != EventDispositionDrop {
		t.Fatalf("Disposition(paused after confirm) = %d, want drop", got)
	}
	if got := store.Disposition(2); got != EventDispositionAllow {
		t.Fatalf("Disposition(candidate after confirm) = %d, want allow", got)
	}
}

func TestStoreResolveIgnoresNonPendingCandidate(t *testing.T) {
	store := NewStore()
	store.Next()
	store.BeginInterruption()

	if store.ResumeIfPending(99) {
		t.Fatal("ResumeIfPending(99) = true, want false")
	}
	if store.ConfirmIfPending(99) {
		t.Fatal("ConfirmIfPending(99) = true, want false")
	}
	if got := store.Current(); got != 2 {
		t.Fatalf("Current() = %d, want 2", got)
	}
	if _, _, ok := store.Pending(); !ok {
		t.Fatal("Pending() ok = false, want true")
	}
}

func TestStoreSubscribeNotifiesOnStateChange(t *testing.T) {
	store := NewStore()
	updates, unsubscribe := store.Subscribe()
	defer unsubscribe()

	store.Next()
	select {
	case <-updates:
	default:
		t.Fatal("missing notification after Next")
	}

	store.BeginInterruption()
	select {
	case <-updates:
	default:
		t.Fatal("missing notification after BeginInterruption")
	}

	store.ResumeIfPending(2)
	select {
	case <-updates:
	default:
		t.Fatal("missing notification after ResumeIfPending")
	}
}
