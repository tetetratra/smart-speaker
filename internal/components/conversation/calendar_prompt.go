package conversation

import (
	"strings"
	"time"

	calendarapi "smart-speaker/internal/googlecalendar"
)

func formatCalendarPrompt(events []calendarapi.Event, day0 time.Time) string {
	labels := []string{"今日", "明日", "明後日"}
	grouped := make([][]string, calendarPromptDays)
	dayIndex := make(map[string]int, calendarPromptDays)
	for i := 0; i < calendarPromptDays; i++ {
		d := day0.AddDate(0, 0, i)
		dayIndex[d.Format("2006-01-02")] = i
	}
	for _, event := range events {
		startAt, ok := eventStartTime(event.Start)
		if !ok {
			continue
		}
		idx, ok := dayIndex[startAt.Format("2006-01-02")]
		if !ok {
			continue
		}
		grouped[idx] = append(grouped[idx], formatCalendarEventLine(event))
	}
	var b strings.Builder
	b.WriteString(calendarPromptPrefix)
	for i := 0; i < calendarPromptDays; i++ {
		b.WriteString("[")
		b.WriteString(labels[i])
		b.WriteString("]\n")
		lines := grouped[i]
		if len(lines) == 0 {
			b.WriteString("- 予定なし\n")
		} else {
			for _, line := range lines {
				b.WriteString("- ")
				b.WriteString(line)
				b.WriteString("\n")
			}
		}
		if i < calendarPromptDays-1 {
			b.WriteString("\n")
		}
	}
	return strings.TrimSpace(b.String())
}

func formatCalendarEventLine(event calendarapi.Event) string {
	title := strings.TrimSpace(event.Summary)
	if title == "" {
		title = "(タイトルなし)"
	}
	start := formatCalendarEventClock(event.Start, false)
	end := formatCalendarEventClock(event.End, true)
	if start == "" && end == "" {
		return title
	}
	if end == "" {
		return strings.TrimSpace(start + " " + title)
	}
	return strings.TrimSpace(start + "-" + end + " " + title)
}

func eventStartTime(start calendarapi.EventTime) (time.Time, bool) {
	if start.DateTime != "" {
		t, err := time.Parse(time.RFC3339, start.DateTime)
		if err != nil {
			return time.Time{}, false
		}
		local := t.In(time.Local)
		return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.Local), true
	}
	if start.Date != "" {
		t, err := time.ParseInLocation("2006-01-02", start.Date, time.Local)
		if err != nil {
			return time.Time{}, false
		}
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local), true
	}
	return time.Time{}, false
}

func formatCalendarEventClock(dt calendarapi.EventTime, isEnd bool) string {
	if dt.Date != "" && dt.DateTime == "" {
		if isEnd {
			return ""
		}
		return "終日"
	}
	if dt.DateTime != "" {
		if t, err := time.Parse(time.RFC3339, dt.DateTime); err == nil {
			return t.In(time.Local).Format("15:04")
		}
	}
	return ""
}
