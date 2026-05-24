package llm

import (
	"encoding/json"
	"fmt"
	"strings"

	types "github.com/tetetratra/smart-speaker/internal/types"
)

type rawTimelineItem struct {
	Type string          `json:"type"`
	Text string          `json:"text"`
	Sec  *float64        `json:"sec"`
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

type rawTimeline struct {
	Items []rawTimelineItem `json:"items"`
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

func (e *timelineParseError) RawPreview() string {
	return e.rawLine
}

func newTimelineParseError(rawPreview string, format string, args ...any) error {
	return &timelineParseError{
		err:     fmt.Errorf(format, args...),
		rawLine: rawPreview,
	}
}

func parseTimelineJSON(rawText string, generationID types.GenerationID) ([]types.TimelineItem, error) {
	rawText = strings.TrimSpace(rawText)
	if rawText == "" {
		return nil, fmt.Errorf("timeline is empty")
	}
	var timeline rawTimeline
	if err := json.Unmarshal([]byte(rawText), &timeline); err != nil {
		return nil, newTimelineParseError(rawText, "invalid timeline json: %w", err)
	}
	items := make([]types.TimelineItem, 0, len(timeline.Items))
	seenTool := false
	for i, raw := range timeline.Items {
		rawPreview := marshalRawTimelineItem(raw)
		if seenTool {
			return nil, newTimelineParseError(rawPreview, "tool must be the last item")
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
				return nil, newTimelineParseError(rawPreview, "speech text is required")
			}
		case types.TimelineKindWait:
			if raw.Sec == nil {
				return nil, newTimelineParseError(rawPreview, "wait sec is required")
			}
			if *raw.Sec < 0 {
				return nil, newTimelineParseError(rawPreview, "wait sec must be non-negative")
			}
			item.Sec = *raw.Sec
		case types.TimelineKindTool:
			item.ToolName = strings.TrimSpace(raw.Name)
			if item.ToolName == "" {
				return nil, newTimelineParseError(rawPreview, "tool name is required")
			}
			if len(raw.Args) == 0 {
				raw.Args = json.RawMessage(`{}`)
			}
			item.ToolArgs = raw.Args
			seenTool = true
		default:
			return nil, newTimelineParseError(rawPreview, "unknown timeline item type: %s", item.Kind)
		}
		items = append(items, item)
	}
	return items, nil
}

func marshalRawTimelineItem(item rawTimelineItem) string {
	type preview struct {
		Type string          `json:"type"`
		Text *string         `json:"text,omitempty"`
		Sec  *float64        `json:"sec,omitempty"`
		Name string          `json:"name,omitempty"`
		Args json.RawMessage `json:"args,omitempty"`
	}
	value := preview{Type: item.Type}
	switch strings.TrimSpace(item.Type) {
	case types.TimelineKindSpeech:
		value.Text = &item.Text
	case types.TimelineKindWait:
		value.Sec = item.Sec
	case types.TimelineKindTool:
		value.Name = item.Name
		if len(item.Args) > 0 {
			value.Args = json.RawMessage(item.Args)
		}
	default:
		if item.Text != "" {
			value.Text = &item.Text
		}
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%+v", item)
	}
	return string(data)
}
