package conversationhistory

import (
	"path/filepath"
	"testing"

	types "github.com/tetetratra/smart-speaker/internal/types"
)

func newTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "conversation_history.json")
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	return store, path
}

func TestStoreUpsertAndSnapshot(t *testing.T) {
	store, _ := newTestStore(t)
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
}

func TestStoreKeepsRecordsAcrossRestartAndRemovesOnlyGivenIDs(t *testing.T) {
	store, path := newTestStore(t)
	for _, id := range []string{"1", "2", "3"} {
		if err := store.Upsert(types.ConversationRecord{ID: id, Text: "発言" + id}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Remove([]string{"1", "3"}); err != nil {
		t.Fatal(err)
	}

	reloaded, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.Snapshot(); len(got) != 1 || got[0].ID != "2" || got[0].Text != "発言2" {
		t.Fatalf("reloaded = %#v", got)
	}
}
