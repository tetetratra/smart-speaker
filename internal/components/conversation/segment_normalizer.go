package conversation

import (
	"regexp"
	"strings"
	"time"
)

var (
	markdownLinkPattern = regexp.MustCompile(`\[[^\]]*\]\((https?://[^)]+)\)`)
	bareURLPattern      = regexp.MustCompile(`https?://\S+`)
	citationPattern     = regexp.MustCompile("cite[^]+")
)

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
