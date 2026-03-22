package conversation

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"
)

var (
	markdownLinkPattern = regexp.MustCompile(`\[[^\]]*\]\((https?://[^)]+)\)`)
	bareURLPattern      = regexp.MustCompile(`https?://\S+`)
	citationPattern     = regexp.MustCompile("cite[^]+")
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

func postWaitDelay(value float64) time.Duration {
	if value <= 0 {
		return 0
	}
	return time.Duration(value * float64(time.Second))
}

func sanitizeSpeech(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	out := markdownLinkPattern.ReplaceAllString(trimmed, "")
	out = bareURLPattern.ReplaceAllString(out, "")
	out = citationPattern.ReplaceAllString(out, "")
	return strings.TrimSpace(out)
}

func sanitizeWait(value *int) int {
	if value == nil {
		return 0
	}
	if *value < 0 {
		return 0
	}
	if *value > 5 {
		return 5
	}
	return *value
}

func waitDelay(value *int) time.Duration {
	return postWaitDelay(normalizeWaitSeconds(value))
}

func normalizeWaitSeconds(value *int) float64 {
	if value == nil {
		return 0
	}
	sec := sanitizeWait(value)
	return float64(sec)
}

func utteranceSource(_ *Utterance) string {
	return "conversation"
}
