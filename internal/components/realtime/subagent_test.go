package realtime

import (
	"strings"
	"testing"
)

func TestSubagentInstructions(t *testing.T) {
	if got := withSubagentInstructions("既存", nil); got != "既存" {
		t.Fatal(got)
	}
	got := withSubagentInstructions("既存", []any{map[string]any{"name": "ask_subagent"}})
	for _, part := range []string{"既存", "了承を得てから", "仲介役", "summary", "details", "set_whiteboard", "全文を読み上げない"} {
		if !strings.Contains(got, part) {
			t.Fatal(part, got)
		}
	}
}
