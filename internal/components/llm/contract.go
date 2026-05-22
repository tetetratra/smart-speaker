package llm

import (
	"encoding/json"
	"fmt"
	"strings"

	types "smart-speaker/internal/types"
)

type rawTimelineItem struct {
	Type string          `json:"type"`
	Text string          `json:"text"`
	Sec  *float64        `json:"sec"`
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

type timelineParseError struct {
	err     error
	rawLine string
}

func (e *timelineParseError) Error() string {
	return e.err.Error()
}

func (e *timelineParseError) Unwrap() error {
	return e.err
}

func (e *timelineParseError) RawLine() string {
	return e.rawLine
}

func newTimelineParseError(rawLine string, format string, args ...any) error {
	return &timelineParseError{
		err:     fmt.Errorf(format, args...),
		rawLine: rawLine,
	}
}

func parseTimeline(lines []string, generationID types.GenerationID) ([]types.TimelineItem, error) {
	items := make([]types.TimelineItem, 0, len(lines))
	seenTool := false
	for i, line := range lines {
		rawLine := line
		line = strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		if seenTool {
			return nil, newTimelineParseError(rawLine, "tool must be the last item")
		}
		var raw rawTimelineItem
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			return nil, newTimelineParseError(rawLine, "invalid ndjson: %w", err)
		}
		item := types.TimelineItem{
			Kind:         strings.TrimSpace(raw.Type),
			GenerationID: generationID,
			SequenceID:   fmt.Sprintf("%d", i+1),
		}
		switch item.Kind {
		case types.TimelineKindSpeech:
			item.Text = strings.TrimSpace(raw.Text)
			if item.Text == "" {
				return nil, newTimelineParseError(rawLine, "speech text is required")
			}
		case types.TimelineKindWait:
			if raw.Sec == nil {
				return nil, newTimelineParseError(rawLine, "wait sec is required")
			}
			if *raw.Sec < 0 {
				return nil, newTimelineParseError(rawLine, "wait sec must be non-negative")
			}
			item.Sec = *raw.Sec
		case types.TimelineKindTool:
			item.ToolName = strings.TrimSpace(raw.Name)
			if item.ToolName == "" {
				return nil, newTimelineParseError(rawLine, "tool name is required")
			}
			if len(raw.Args) == 0 {
				raw.Args = json.RawMessage(`{}`)
			}
			item.ToolArgs = raw.Args
			seenTool = true
		default:
			return nil, newTimelineParseError(rawLine, "unknown timeline item type: %s", item.Kind)
		}
		items = append(items, item)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("timeline is empty")
	}
	return items, nil
}
