package memory

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	memoryhook "github.com/tetetratra/smart-speaker/internal/hooks/memory"
	memorystate "github.com/tetetratra/smart-speaker/internal/states/memory"
)

func newStore(t *testing.T) *memorystate.Store {
	t.Helper()
	store, err := memorystate.NewStore(filepath.Join(t.TempDir(), "memories.json"))
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func run(t *testing.T, store *memorystate.Store, operation string, args map[string]any) (map[string]any, error) {
	t.Helper()
	return (&Tool{Store: store, Operation: operation}).Run(context.Background(), args)
}

// 会話から追加・一覧・変更・削除ができ、画面と同じ保存先に反映される。
func TestMemoryToolsChangeStore(t *testing.T) {
	store := newStore(t)

	added, err := run(t, store, "add_memory", map[string]any{"content": "ユーザーの長女の誕生日は5月3日"})
	if err != nil || added["created"] != true {
		t.Fatal(added, err)
	}
	id := added["memory"].(map[string]any)["id"].(string)

	// 同じ本文は重複して追加しない。
	again, err := run(t, store, "add_memory", map[string]any{"content": " ユーザーの長女の誕生日は5月3日 "})
	if err != nil || again["created"] != false || len(store.Snapshot()) != 1 {
		t.Fatal(again, err)
	}

	listed, err := run(t, store, "list_memories", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	items := listed["memories"].([]map[string]any)
	if len(items) != 1 || items[0]["id"] != id || items[0]["content"] != "ユーザーの長女の誕生日は5月3日" {
		t.Fatal(items)
	}

	if _, err := run(t, store, "update_memory", map[string]any{"id": id, "content": "ユーザーの長女の誕生日は5月4日"}); err != nil {
		t.Fatal(err)
	}
	if got := store.Snapshot()[0].Content; got != "ユーザーの長女の誕生日は5月4日" {
		t.Fatal(got)
	}

	deleted, err := run(t, store, "delete_memory", map[string]any{"id": id})
	if err != nil || deleted["deleted"].(map[string]any)["content"] != "ユーザーの長女の誕生日は5月4日" {
		t.Fatal(deleted, err)
	}
	if len(store.Snapshot()) != 0 {
		t.Fatal(store.Snapshot())
	}
}

// 失敗したときは、AIが次にすべきことが分かる言葉で返し、保存内容は変えない。
func TestMemoryToolsRejectInvalidRequests(t *testing.T) {
	store := newStore(t)
	if _, err := run(t, store, "add_memory", map[string]any{"content": "残すメモ"}); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		operation string
		args      map[string]any
		want      string
	}{
		{"add_memory", map[string]any{"content": "  "}, "contentが空"},
		{"update_memory", map[string]any{"id": "missing", "content": "x"}, "list_memories"},
		{"delete_memory", map[string]any{"id": "missing"}, "list_memories"},
		{"delete_memory", map[string]any{"id": ""}, "list_memories"},
		{"add_memory", map[string]any{"content": "x", "tags": []any{"a"}}, "引数"},
	}
	for _, c := range cases {
		_, err := run(t, store, c.operation, c.args)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Fatalf("%s %v: err=%v", c.operation, c.args, err)
		}
	}
	if records := store.Snapshot(); len(records) != 1 || records[0].Content != "残すメモ" {
		t.Fatal(records)
	}
}

// 一覧以外の3つには、ユーザーの依頼か了承を求める説明が付く。
func TestMemoryToolDefinitions(t *testing.T) {
	for _, operation := range Operations {
		def := (&Tool{Operation: operation}).Definition()
		if def["name"] != operation {
			t.Fatal(def["name"])
		}
		description := def["description"].(string)
		if !strings.Contains(description, memoryRole) {
			t.Fatal(operation, "メモリの役割の説明がありません")
		}
		if operation == "list_memories" {
			continue
		}
		// 何を残す価値があるかは、会話終了時の自動抽出と同じ基準を示す。
		if !strings.Contains(description, "了承") || !strings.Contains(description, memoryhook.RetentionCriteria) {
			t.Fatal(operation, description)
		}
	}
}
