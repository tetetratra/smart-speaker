package llm

import (
	"errors"
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
	rawLine := `{"type":"speech","text":"結果です"}`
	_, err := parseTimeline([]string{
		`{"type":"tool","name":"get_temp","args":{}}`,
		rawLine,
	}, 1)
	if err == nil {
		t.Fatal("err = nil, want error")
	}
	var parseErr *timelineParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("err = %T, want timelineParseError", err)
	}
	if parseErr.RawLine() != rawLine {
		t.Fatalf("RawLine = %q, want %q", parseErr.RawLine(), rawLine)
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

func TestParseTimelineErrorKeepsRawLine(t *testing.T) {
	cases := []struct {
		name string
		line string
	}{
		{name: "invalid json", line: `通常文の応答です`},
		{name: "empty speech text", line: `{"type":"speech","text":"   "}`},
		{name: "unknown type", line: `{"type":"other","text":"確認します"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseTimeline([]string{tc.line}, 1)
			if err == nil {
				t.Fatal("err = nil, want error")
			}
			var parseErr *timelineParseError
			if !errors.As(err, &parseErr) {
				t.Fatalf("err = %T, want timelineParseError", err)
			}
			if parseErr.RawLine() != tc.line {
				t.Fatalf("RawLine = %q, want %q", parseErr.RawLine(), tc.line)
			}
		})
	}
}

func TestParseTimelineEmptyTimelineHasNoRawLine(t *testing.T) {
	_, err := parseTimeline([]string{"", "   "}, 1)
	if err == nil {
		t.Fatal("err = nil, want error")
	}
	var parseErr *timelineParseError
	if errors.As(err, &parseErr) {
		t.Fatalf("timelineParseError.RawLine = %q, want no raw line", parseErr.RawLine())
	}
}
