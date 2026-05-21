package conversation

import (
	"encoding/json"
	"strings"
)

type aiOutput struct {
	Timeline []aiSegment `json:"timeline"`
}

type aiSegment struct {
	Type string          `json:"type"`
	Sec  *int            `json:"sec,omitempty"`
	Text string          `json:"text,omitempty"`
	Name string          `json:"name,omitempty"`
	Args json.RawMessage `json:"args,omitempty"`
}

type aiChunk struct {
	Type string          `json:"type"`
	Sec  *int            `json:"sec,omitempty"`
	Text string          `json:"text,omitempty"`
	Name string          `json:"name,omitempty"`
	Args json.RawMessage `json:"args,omitempty"`
}

func parseAIOutput(raw string) (aiOutput, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return aiOutput{}, false
	}
	chunks, ok := parseAIChunks(trimmed)
	if !ok {
		return aiOutput{}, false
	}
	return buildAIOutputFromChunks(chunks)
}

func validateAIOutput(out aiOutput) (aiOutput, bool) {
	if len(out.Timeline) == 0 {
		return aiOutput{}, false
	}
	speechCount := 0
	toolSeen := false
	for _, seg := range out.Timeline {
		if toolSeen {
			return aiOutput{}, false
		}
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
		case "tool":
			if strings.TrimSpace(seg.Name) == "" {
				return aiOutput{}, false
			}
			toolSeen = true
		default:
			return aiOutput{}, false
		}
	}
	if speechCount == 0 && !toolSeen {
		return aiOutput{}, false
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
	case "tool":
		chunk.Name = strings.TrimSpace(chunk.Name)
		if chunk.Name == "" {
			return aiChunk{}, false
		}
		if len(chunk.Args) == 0 {
			chunk.Args = json.RawMessage(`{}`)
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

	lines := strings.Split(trimmed, "\n")
	chunks := make([]aiChunk, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !tryAppendChunkObject(&chunks, line) {
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
		case "tool":
			out.Timeline = append(out.Timeline, aiSegment{Type: "tool", Name: chunk.Name, Args: chunk.Args})
		default:
			return aiOutput{}, false
		}
	}
	return validateAIOutput(out)
}
