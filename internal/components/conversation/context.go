package conversation

import (
	"context"
	"log"
	"strings"
	"time"

	diarystore "smart-speaker/internal/diary"
	calendarapi "smart-speaker/internal/googlecalendar"
	oauthgooglecalendar "smart-speaker/internal/oauth/googlecalendar"
	types "smart-speaker/internal/types"
)

type contextProvider struct {
	calendarClient calendarEventLister
	diaryReader    DiaryReader
}

type calendarEventLister interface {
	ListEvents(ctx context.Context, req calendarapi.ListEventsRequest) ([]calendarapi.Event, error)
}

type DiaryReader interface {
	Content() (string, error)
}

// contextProviderを依存未指定時のdefault実装つきで作る。
func newContextProvider(client calendarEventLister, reader DiaryReader) *contextProvider {
	if client == nil {
		client = calendarapi.NewClient(calendarapi.Config{})
	}
	if reader == nil {
		reader = diarystore.NewStore(diarystore.Config{})
	}
	return &contextProvider{
		calendarClient: client,
		diaryReader:    reader,
	}
}

// 会話messagesの先頭にカレンダーと日記のsystem contextを追加する。
func (p *contextProvider) WithSystemContexts(ctx context.Context, messages []types.ChatMessage) []types.ChatMessage {
	out := p.withCalendarContext(ctx, messages)
	return p.withDiaryContext(out)
}

// カレンダー予定をsystem messageとしてmessagesの先頭へ追加する。
func (p *contextProvider) withCalendarContext(ctx context.Context, messages []types.ChatMessage) []types.ChatMessage {
	built, err := p.buildCalendarContext(ctx)
	if err != nil {
		log.Printf("conversation: failed to build calendar context: %v", err)
		return messages
	}
	content := strings.TrimSpace(built)
	if content == "" {
		return messages
	}
	withCalendar := make([]types.ChatMessage, 0, len(messages)+1)
	withCalendar = append(withCalendar, types.ChatMessage{
		Role:    "system",
		Content: content,
	})
	withCalendar = append(withCalendar, messages...)
	return withCalendar
}

// Google Calendarから直近予定のprompt本文を組み立てる。
func (p *contextProvider) buildCalendarContext(ctx context.Context) (string, error) {
	if _, err := oauthgooglecalendar.LoadToken(); err != nil {
		return "", nil
	}
	if ctx == nil {
		return "", nil
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	now := time.Now()
	day0 := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	dayN := day0.AddDate(0, 0, calendarPromptDays)
	return buildCalendarContextWithClient(ctx, p.calendarClient, day0, dayN)
}

// 日記本文をsystem messageとしてmessagesの先頭へ追加する。
func (p *contextProvider) withDiaryContext(messages []types.ChatMessage) []types.ChatMessage {
	if p == nil || p.diaryReader == nil {
		return messages
	}
	diary, err := p.diaryReader.Content()
	if err != nil {
		log.Printf("conversation: failed to read diary context: %v", err)
		return messages
	}
	diary = strings.TrimSpace(diary)
	if diary == "" {
		return messages
	}
	withDiary := make([]types.ChatMessage, 0, len(messages)+1)
	withDiary = append(withDiary, types.ChatMessage{
		Role:    "system",
		Content: diaryPromptPrefix + diary,
	})
	withDiary = append(withDiary, messages...)
	return withDiary
}

// 指定期間のカレンダー予定を取得してprompt本文へ整形する。
func buildCalendarContextWithClient(ctx context.Context, client calendarEventLister, day0 time.Time, dayN time.Time) (string, error) {
	events, err := client.ListEvents(ctx, calendarapi.ListEventsRequest{
		CalendarID: "primary",
		TimeMin:    day0,
		TimeMax:    dayN,
		MaxResults: calendarFetchMaxResults,
	})
	if err != nil {
		return "", err
	}
	return formatCalendarPrompt(events, day0), nil
}

// カレンダー予定を日別の箇条書きpromptへ整形する。
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

// カレンダー予定1件を時刻つきの1行表示へ整形する。
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

// 予定開始日をローカル日付の0時として取り出す。
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

// カレンダー予定時刻をHH:MMまたは終日表記へ整形する。
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
