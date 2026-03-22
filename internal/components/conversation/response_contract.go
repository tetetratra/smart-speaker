package conversation

import (
	"encoding/json"
	"strings"
)

type aiOutput struct {
	Timeline   []aiSegment   `json:"timeline"`
	Whiteboard *aiWhiteboard `json:"whiteboard,omitempty"`
}

type aiWhiteboard struct {
	Content string `json:"content"`
}

type aiSegment struct {
	Type string `json:"type"`
	Sec  *int   `json:"sec,omitempty"`
	Text string `json:"text,omitempty"`
}

func parseAIOutput(raw string) (aiOutput, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return aiOutput{}, false
	}
	var out aiOutput
	dec := json.NewDecoder(strings.NewReader(trimmed))
	if err := dec.Decode(&out); err != nil {
		return aiOutput{}, false
	}
	if len(out.Timeline) == 0 {
		return aiOutput{}, false
	}
	speechCount := 0
	for _, seg := range out.Timeline {
		switch seg.Type {
		case "wait":
			if seg.Sec == nil {
				return aiOutput{}, false
			}
		case "speech":
			if strings.TrimSpace(seg.Text) == "" {
				return aiOutput{}, false
			}
			speechCount++
		default:
			return aiOutput{}, false
		}
	}
	if speechCount == 0 {
		return aiOutput{}, false
	}
	if out.Whiteboard != nil {
		out.Whiteboard.Content = strings.TrimSpace(out.Whiteboard.Content)
		if out.Whiteboard.Content == "" {
			return aiOutput{}, false
		}
	}
	return out, true
}
