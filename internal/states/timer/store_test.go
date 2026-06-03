package timer

import (
	"testing"
	"time"
)

func TestStoreCreateSnapshotCancel(t *testing.T) {
	now := time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC)
	store := NewStoreWithClock(func() time.Time { return now })

	later := now.Add(30 * time.Minute)
	earlier := now.Add(10 * time.Minute)
	first := store.Create(later, "  エアコンをoffにする  ")
	second := store.Create(earlier, "起こす")

	items := store.Snapshot()
	if len(items) != 2 {
		t.Fatalf("Snapshot len = %d, want 2", len(items))
	}
	if items[0].ID != second.ID || items[1].ID != first.ID {
		t.Fatalf("Snapshot order = [%s, %s], want [%s, %s]", items[0].ID, items[1].ID, second.ID, first.ID)
	}
	if first.Action != "エアコンをoffにする" {
		t.Fatalf("Action = %q", first.Action)
	}

	cancelled, ok := store.Cancel(first.ID)
	if !ok {
		t.Fatal("Cancel returned ok=false")
	}
	if cancelled.ID != first.ID {
		t.Fatalf("cancelled ID = %q, want %q", cancelled.ID, first.ID)
	}
	if got := len(store.Snapshot()); got != 1 {
		t.Fatalf("Snapshot len after cancel = %d, want 1", got)
	}
}

func TestStorePopDueRemovesDueTimers(t *testing.T) {
	now := time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC)
	store := NewStoreWithClock(func() time.Time { return now })
	due := store.Create(now.Add(-time.Second), "期限到達")
	future := store.Create(now.Add(time.Minute), "まだ")

	items := store.PopDue(now)
	if len(items) != 1 || items[0].ID != due.ID {
		t.Fatalf("PopDue = %#v, want due timer %s", items, due.ID)
	}
	remaining := store.Snapshot()
	if len(remaining) != 1 || remaining[0].ID != future.ID {
		t.Fatalf("remaining = %#v, want future timer %s", remaining, future.ID)
	}
}
