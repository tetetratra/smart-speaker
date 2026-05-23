package llm

import (
	"errors"
	"testing"

	types "smart-speaker/internal/types"
)

func TestParseTimelineJSONAcceptsSpeechWaitAndTrailingTool(t *testing.T) {
	items, err := parseTimelineJSON(`{"items":[{"type":"speech","text":"確認します"},{"type":"wait","sec":0.5},{"type":"tool","name":"get_temp","args":{"room":"living"}}]}`, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("len = %d, want 3", len(items))
	}
	if items[0].SequenceID != "1" || items[1].SequenceID != "2" || items[2].SequenceID != "3" {
		t.Fatalf("items = %+v", items)
	}
	if items[2].Kind != types.TimelineKindTool || items[2].ToolName != "get_temp" {
		t.Fatalf("tool item = %+v", items[2])
	}
	if string(items[2].ToolArgs) != `{"room":"living"}` {
		t.Fatalf("ToolArgs = %s", items[2].ToolArgs)
	}
}

func TestParseTimelineJSONRejectsItemAfterTool(t *testing.T) {
	_, err := parseTimelineJSON(`{"items":[{"type":"tool","name":"get_temp","args":{}},{"type":"speech","text":"結果です"}]}`, 1)
	if err == nil {
		t.Fatal("err = nil, want error")
	}
	var parseErr *timelineParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("err = %T, want timelineParseError", err)
	}
	if parseErr.RawPreview() == "" {
		t.Fatal("RawPreview is empty")
	}
}

func TestParseTimelineJSONRejectsInvalidItems(t *testing.T) {
	cases := []string{
		`{"items":[{"type":"speech","text":""}]}`,
		`{"items":[{"type":"wait"}]}`,
		`{"items":[{"type":"wait","sec":-1}]}`,
		`{"items":[{"type":"tool","name":"","args":{}}]}`,
	}
	for _, raw := range cases {
		if _, err := parseTimelineJSON(raw, 1); err == nil {
			t.Fatalf("parseTimelineJSON(%s) err = nil, want error", raw)
		}
	}
}

func TestParseTimelineJSONErrorKeepsRawPreview(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{name: "invalid json", raw: `通常文の応答です`},
		{name: "empty speech text", raw: `{"items":[{"type":"speech","text":"   "}]}`},
		{name: "unknown type", raw: `{"items":[{"type":"other","text":"確認します"}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseTimelineJSON(tc.raw, 1)
			if err == nil {
				t.Fatal("err = nil, want error")
			}
			var parseErr *timelineParseError
			if !errors.As(err, &parseErr) {
				t.Fatalf("err = %T, want timelineParseError", err)
			}
			if parseErr.RawPreview() == "" {
				t.Fatal("RawPreview is empty")
			}
		})
	}
}

func TestParseTimelineJSONAcceptsEmptyTimeline(t *testing.T) {
	items, err := parseTimelineJSON(`{"items":[]}`, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("len = %d, want 0", len(items))
	}
}
