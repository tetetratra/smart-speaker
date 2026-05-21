package conversation

import (
	"regexp"
	"strings"
)

var (
	markdownLinkPattern = regexp.MustCompile(`\[[^\]]*\]\((https?://[^)]+)\)`)
	bareURLPattern      = regexp.MustCompile(`https?://\S+`)
	citationPattern     = regexp.MustCompile("cite[^]+")
)

func buildTimelineSegments(out aiOutput) []timelineSegment {
	if len(out.Timeline) == 0 {
		return nil
	}
	timeline := make([]timelineSegment, 0, len(out.Timeline))
	speechCount := 0
	for _, seg := range out.Timeline {
		switch seg.Type {
		case "wait":
			timeline = append(timeline, timelineSegment{
				Type:    "wait",
				WaitSec: sanitizeWait(seg.Sec),
			})
		case "speech":
			text := sanitizeSpeech(seg.Text)
			if text == "" {
				continue
			}
			timeline = append(timeline, timelineSegment{Type: "speech", Text: text})
			speechCount++
		case "tool":
			timeline = append(timeline, timelineSegment{
				Type: "tool",
				Tool: &toolCallSegment{
					Name: strings.TrimSpace(seg.Name),
					Args: seg.Args,
				},
			})
		}
	}
	if speechCount == 0 && (len(timeline) == 0 || timeline[len(timeline)-1].Type != "tool") {
		return nil
	}
	return timeline
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
