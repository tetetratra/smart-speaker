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

func parseTimeline(lines []string, generationID types.GenerationID) ([]types.TimelineItem, error) {
	items := make([]types.TimelineItem, 0, len(lines))
	seenTool := false
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if seenTool {
			return nil, fmt.Errorf("tool must be the last item")
		}
		var raw rawTimelineItem
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			return nil, fmt.Errorf("invalid ndjson: %w", err)
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
				return nil, fmt.Errorf("speech text is required")
			}
		case types.TimelineKindWait:
			if raw.Sec == nil {
				return nil, fmt.Errorf("wait sec is required")
			}
			if *raw.Sec < 0 {
				return nil, fmt.Errorf("wait sec must be non-negative")
			}
			item.Sec = *raw.Sec
		case types.TimelineKindTool:
			item.ToolName = strings.TrimSpace(raw.Name)
			if item.ToolName == "" {
				return nil, fmt.Errorf("tool name is required")
			}
			if len(raw.Args) == 0 {
				raw.Args = json.RawMessage(`{}`)
			}
			item.ToolArgs = raw.Args
			seenTool = true
		default:
			return nil, fmt.Errorf("unknown timeline item type: %s", item.Kind)
		}
		items = append(items, item)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("timeline is empty")
	}
	return items, nil
}
