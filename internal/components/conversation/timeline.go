package conversation

import (
	"time"

	types "smart-speaker/internal/types"
)

func buildUtteranceChain(out aiOutput) []aiSegment {
	if len(out.Timeline) == 0 {
		return nil
	}
	timeline := make([]aiSegment, 0, len(out.Timeline))
	speechCount := 0
	for _, seg := range out.Timeline {
		switch seg.Type {
		case "wait":
			wait := sanitizeWait(seg.Sec)
			timeline = append(timeline, aiSegment{Type: "wait", Sec: &wait})
		case "speech":
			text := sanitizeSpeech(seg.Text)
			if text == "" {
				continue
			}
			timeline = append(timeline, aiSegment{Type: "speech", Text: text})
			speechCount++
		}
	}
	if speechCount == 0 {
		return nil
	}
	return timeline
}

func estimateWaitDuration(tts types.TTSEvent, waitSec float64) time.Duration {
	waitDuration := postWaitDelay(waitSec)
	startAt := tts.AudioStartAt
	if startAt.IsZero() {
		return waitDuration
	}
	if tts.DurationSeconds <= 0 {
		return waitDuration
	}
	endAt := startAt.Add(time.Duration(tts.DurationSeconds * float64(time.Second)))
	remaining := time.Until(endAt)
	if remaining < 0 {
		remaining = 0
	}
	return remaining + waitDuration
}
