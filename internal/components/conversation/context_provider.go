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

func (p *contextProvider) WithSystemContexts(ctx context.Context, messages []types.ChatMessage) []types.ChatMessage {
	leadingImportant, rest := splitLeadingImportantSystemMessages(messages)
	out := p.withCalendarContext(ctx, rest)
	out = p.withDiaryContext(out)
	if len(leadingImportant) == 0 {
		return out
	}
	merged := make([]types.ChatMessage, 0, len(leadingImportant)+len(out))
	merged = append(merged, leadingImportant...)
	merged = append(merged, out...)
	return merged
}

func splitLeadingImportantSystemMessages(messages []types.ChatMessage) ([]types.ChatMessage, []types.ChatMessage) {
	idx := 0
	for idx < len(messages) {
		msg := messages[idx]
		if msg.Role != "system" || !strings.HasPrefix(msg.Content, importantRetryPrefix) {
			break
		}
		idx++
	}
	if idx == 0 {
		return nil, messages
	}
	leading := make([]types.ChatMessage, idx)
	copy(leading, messages[:idx])
	return leading, messages[idx:]
}

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
