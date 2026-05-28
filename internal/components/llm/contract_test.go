package llm

import (
	"errors"
	"testing"

	types "github.com/tetetratra/smart-speaker/internal/types"
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

func TestParseTimelineJSONPrependsSetWhiteboardFromRootField(t *testing.T) {
	items, err := parseTimelineJSON(`{"set_whiteboard":{"content":"予定: 10:00 会議"},"items":[{"type":"speech","text":"確認したよ"}]}`, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("len = %d, want 2", len(items))
	}
	if items[0].SequenceID != "1" || items[1].SequenceID != "2" {
		t.Fatalf("sequence ids = %+v", items)
	}
	if items[0].Kind != types.TimelineKindTool || items[0].ToolName != "set_whiteboard" {
		t.Fatalf("first item = %+v", items[0])
	}
	if string(items[0].ToolArgs) != `{"content":"予定: 10:00 会議"}` {
		t.Fatalf("ToolArgs = %s", items[0].ToolArgs)
	}
	if items[1].Kind != types.TimelineKindSpeech {
		t.Fatalf("second item = %+v", items[1])
	}
}

func TestParseTimelineJSONAcceptsSetWhiteboardFieldOnly(t *testing.T) {
	items, err := parseTimelineJSON(`{"set_whiteboard":{"content":"メモ"},"items":[]}`, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("len = %d, want 1", len(items))
	}
	if items[0].ToolName != "set_whiteboard" || items[0].SequenceID != "1" {
		t.Fatalf("item = %+v", items[0])
	}
}

func TestParseTimelineJSONRejectsSetWhiteboardInItems(t *testing.T) {
	cases := []string{
		`{"items":[{"type":"tool","name":"set_whiteboard","args":{"content":"メモ"}}]}`,
		`{"set_whiteboard":{"content":"メモ"},"items":[{"type":"tool","name":"set_whiteboard","args":{"content":"メモ"}}]}`,
	}
	for _, raw := range cases {
		if _, err := parseTimelineJSON(raw, 1); err == nil {
			t.Fatalf("parseTimelineJSON(%s) err = nil, want error", raw)
		}
	}
}

func TestParseTimelineJSONRejectsEmptySetWhiteboardContent(t *testing.T) {
	cases := []string{
		`{"set_whiteboard":{"content":""},"items":[]}`,
		`{"set_whiteboard":{"content":"   "},"items":[]}`,
	}
	for _, raw := range cases {
		if _, err := parseTimelineJSON(raw, 1); err == nil {
			t.Fatalf("parseTimelineJSON(%s) err = nil, want error", raw)
		}
	}
}

func TestParseTimelineJSONAcceptsMultipleTools(t *testing.T) {
	items, err := parseTimelineJSON(`{"items":[{"type":"tool","name":"web_search","args":{"query":"天気"}},{"type":"speech","text":"調べるね"}]}`, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("len = %d, want 2", len(items))
	}
	if items[0].Kind != types.TimelineKindTool || items[0].ToolName != "web_search" {
		t.Fatalf("first tool = %+v", items[0])
	}
	if items[1].Kind != types.TimelineKindSpeech {
		t.Fatalf("second item = %+v", items[1])
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

func TestParseTimelineJSONIgnoresNullSetWhiteboard(t *testing.T) {
	items, err := parseTimelineJSON(`{"set_whiteboard":null,"items":[{"type":"speech","text":"了解"}]}`, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Kind != types.TimelineKindSpeech {
		t.Fatalf("items = %+v", items)
	}
}
