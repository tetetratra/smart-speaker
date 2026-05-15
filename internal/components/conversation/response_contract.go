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

type aiChunk struct {
	Type    string `json:"type"`
	Sec     *int   `json:"sec,omitempty"`
	Text    string `json:"text,omitempty"`
	Content string `json:"content,omitempty"`
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

func parseAIChunk(raw string) (aiChunk, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return aiChunk{}, false
	}
	var chunk aiChunk
	dec := json.NewDecoder(strings.NewReader(trimmed))
	if err := dec.Decode(&chunk); err != nil {
		return aiChunk{}, false
	}
	switch chunk.Type {
	case "speech":
		chunk.Text = strings.TrimSpace(chunk.Text)
		if chunk.Text == "" {
			return aiChunk{}, false
		}
	case "wait":
		if chunk.Sec == nil {
			return aiChunk{}, false
		}
	case "whiteboard":
		chunk.Content = strings.TrimSpace(chunk.Content)
		if chunk.Content == "" {
			return aiChunk{}, false
		}
	default:
		return aiChunk{}, false
	}
	return chunk, true
}
