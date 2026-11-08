package conversationhistory

import (
	"testing"

	types "github.com/tetetratra/smart-speaker/internal/types"
)

func TestStoreUpsertSnapshotAndDrain(t *testing.T) {
	store := NewStore()
	store.Upsert(types.ConversationRecord{ID: "1", Role: types.RoleUser, Text: "こんにちは", Metadata: map[string]any{"k": "v"}})
	store.Upsert(types.ConversationRecord{ID: "2", Role: types.RoleAgent, Text: "はい"})

	snapshot := store.Snapshot()
	if len(snapshot) != 2 {
		t.Fatalf("Snapshot len = %d, want 2", len(snapshot))
	}
	snapshot[0].Text = "changed"
	snapshot[0].Metadata["k"] = "changed"

	again := store.Snapshot()
	if again[0].Text != "こんにちは" {
		t.Fatalf("record text changed through snapshot: %q", again[0].Text)
	}
	if again[0].Metadata["k"] != "v" {
		t.Fatalf("metadata changed through snapshot: %v", again[0].Metadata["k"])
	}

	store.Upsert(types.ConversationRecord{ID: "1", Role: types.RoleUser, Text: "こんにちは！"})
	if got := store.Snapshot(); len(got) != 2 || got[0].Text != "こんにちは！" {
		t.Fatalf("Upsert did not replace the record in place: %#v", got)
	}

	if drained := store.Drain(); len(drained) != 2 {
		t.Fatalf("Drain len = %d, want 2", len(drained))
	}
	if got := len(store.Snapshot()); got != 0 {
		t.Fatalf("Snapshot len after Drain = %d, want 0", got)
	}
}
