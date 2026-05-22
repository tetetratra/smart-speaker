package llm

import (
	"testing"

	types "smart-speaker/internal/types"
)

func TestParseTimelineAcceptsSpeechWaitAndTrailingTool(t *testing.T) {
	items, err := parseTimeline([]string{
		`{"type":"speech","text":"確認します"}`,
		`{"type":"wait","sec":0.5}`,
		`{"type":"tool","name":"get_temp","args":{"room":"living"}}`,
	}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("len = %d, want 3", len(items))
	}
	if items[2].Kind != types.TimelineKindTool || items[2].ToolName != "get_temp" {
		t.Fatalf("tool item = %+v", items[2])
	}
}

func TestParseTimelineRejectsItemAfterTool(t *testing.T) {
	_, err := parseTimeline([]string{
		`{"type":"tool","name":"get_temp","args":{}}`,
		`{"type":"speech","text":"結果です"}`,
	}, 1)
	if err == nil {
		t.Fatal("err = nil, want error")
	}
}

func TestParseTimelineRejectsInvalidItems(t *testing.T) {
	cases := [][]string{
		{`{"type":"speech","text":""}`},
		{`{"type":"wait"}`},
		{`{"type":"tool","args":{}}`},
	}
	for _, lines := range cases {
		if _, err := parseTimeline(lines, 1); err == nil {
			t.Fatalf("parseTimeline(%v) err = nil, want error", lines)
		}
	}
}
