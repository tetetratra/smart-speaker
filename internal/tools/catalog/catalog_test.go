package catalog

import (
	"context"
	"testing"

	"github.com/tetetratra/smart-speaker/internal/tools"
)

type namedTool string

func (t namedTool) Name() string { return string(t) }
func (t namedTool) Run(context.Context, map[string]any) (map[string]any, error) {
	return map[string]any{"tool": string(t)}, nil
}

func TestRebuildReplacesSnapshotAndNotifies(t *testing.T) {
	names := []string{"a"}
	c := New(func() Snapshot {
		handlers := map[string]tools.Handler{}
		var defs []any
		for _, n := range names {
			handlers[n] = namedTool(n)
			defs = append(defs, map[string]any{"name": n})
		}
		return Snapshot{Definitions: defs, Handlers: handlers, Instructions: "tools=" + names[len(names)-1]}
	})
	before := c.Current()
	notified := 0
	c.OnChange(func() {
		notified++
		if c.Handlers()["b"] == nil {
			t.Error("知らせる時点で、一覧が差し替わっていません")
		}
	})

	names = append(names, "b")
	c.Rebuild()

	if notified != 1 {
		t.Fatalf("notified = %d", notified)
	}
	if len(c.Definitions()) != 2 || c.Instructions() != "tools=b" {
		t.Fatalf("差し替えたあと = %+v", c.Current())
	}
	// 前に読んだ一覧は変わらない。
	if len(before.Definitions) != 1 || before.Handlers["b"] != nil {
		t.Fatalf("前の一覧が変わりました: %+v", before)
	}
}

func TestStaticHasEmptyHandlers(t *testing.T) {
	c := Static(Snapshot{})
	if c.Handlers() == nil {
		t.Fatal("Handlers が nil です")
	}
}
