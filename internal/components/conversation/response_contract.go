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
	if out, ok := parseAIStructuredOutput(trimmed); ok {
		return out, true
	}
	chunks, ok := parseAIChunks(trimmed)
	if !ok {
		return aiOutput{}, false
	}
	return buildAIOutputFromChunks(chunks)
}

func parseAIStructuredOutput(raw string) (aiOutput, bool) {
	var out aiOutput
	dec := json.NewDecoder(strings.NewReader(raw))
	if err := dec.Decode(&out); err != nil {
		return aiOutput{}, false
	}
	var extra any
	if err := dec.Decode(&extra); err == nil {
		return aiOutput{}, false
	}
	return validateAIOutput(out)
}

func validateAIOutput(out aiOutput) (aiOutput, bool) {
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
	return parseAIChunkObject(trimmed)
}

func parseAIChunkObject(raw string) (aiChunk, bool) {
	var chunk aiChunk
	dec := json.NewDecoder(strings.NewReader(raw))
	if err := dec.Decode(&chunk); err != nil {
		return aiChunk{}, false
	}
	var extra any
	if err := dec.Decode(&extra); err == nil {
		return aiChunk{}, false
	}
	return validateAIChunk(chunk)
}

func validateAIChunk(chunk aiChunk) (aiChunk, bool) {
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

func parseAIChunks(raw string) ([]aiChunk, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, false
	}
	if chunk, ok := parseAIChunkObject(trimmed); ok {
		return []aiChunk{chunk}, true
	}
	if out, ok := parseAIStructuredOutput(trimmed); ok {
		return flattenAIOutput(out), true
	}

	lines := strings.Split(trimmed, "\n")
	chunks := make([]aiChunk, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		switch {
		case tryAppendChunkObject(&chunks, line):
		case tryAppendStructuredOutput(&chunks, line):
		default:
			return nil, false
		}
	}
	if len(chunks) == 0 {
		return nil, false
	}
	return chunks, true
}

func tryAppendChunkObject(chunks *[]aiChunk, raw string) bool {
	chunk, ok := parseAIChunkObject(raw)
	if !ok {
		return false
	}
	*chunks = append(*chunks, chunk)
	return true
}

func tryAppendStructuredOutput(chunks *[]aiChunk, raw string) bool {
	out, ok := parseAIStructuredOutput(raw)
	if !ok {
		return false
	}
	*chunks = append(*chunks, flattenAIOutput(out)...)
	return true
}

func flattenAIOutput(out aiOutput) []aiChunk {
	chunks := make([]aiChunk, 0, len(out.Timeline)+1)
	for _, seg := range out.Timeline {
		chunks = append(chunks, aiChunk{
			Type: seg.Type,
			Sec:  seg.Sec,
			Text: seg.Text,
		})
	}
	if out.Whiteboard != nil {
		chunks = append(chunks, aiChunk{
			Type:    "whiteboard",
			Content: out.Whiteboard.Content,
		})
	}
	return chunks
}

func buildAIOutputFromChunks(chunks []aiChunk) (aiOutput, bool) {
	out := aiOutput{
		Timeline: make([]aiSegment, 0, len(chunks)),
	}
	for _, chunk := range chunks {
		switch chunk.Type {
		case "speech":
			out.Timeline = append(out.Timeline, aiSegment{Type: "speech", Text: chunk.Text})
		case "wait":
			out.Timeline = append(out.Timeline, aiSegment{Type: "wait", Sec: chunk.Sec})
		case "whiteboard":
			if out.Whiteboard == nil {
				out.Whiteboard = &aiWhiteboard{}
			}
			if out.Whiteboard.Content == "" {
				out.Whiteboard.Content = chunk.Content
			}
		default:
			return aiOutput{}, false
		}
	}
	return validateAIOutput(out)
}
