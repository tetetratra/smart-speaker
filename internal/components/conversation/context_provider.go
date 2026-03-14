package conversation

import (
	"context"
	"log"
	"strings"
	"time"

	oauthgooglecalendar "smart-speaker/internal/oauth/googlecalendar"
	"smart-speaker/internal/state"
	types "smart-speaker/internal/types"
)

type contextProvider struct {
	calendarContextCache string
}

func newContextProvider() *contextProvider {
	return &contextProvider{}
}

func (p *contextProvider) WithSystemContexts(ctx context.Context, messages []types.ChatMessage) []types.ChatMessage {
	out := p.withCalendarContext(ctx, messages)
	return withDiaryContext(out)
}

func (p *contextProvider) withCalendarContext(ctx context.Context, messages []types.ChatMessage) []types.ChatMessage {
	content := strings.TrimSpace(p.calendarContextCache)
	if content == "" {
		built, err := buildCalendarContext(ctx)
		if err != nil {
			log.Printf("conversation: failed to build calendar context: %v", err)
			return messages
		}
		content = strings.TrimSpace(built)
		p.calendarContextCache = content
	}
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

func (p *contextProvider) InvalidateCalendar() {
	p.calendarContextCache = ""
}

func (p *contextProvider) Clear() {
	p.calendarContextCache = ""
}

func withDiaryContext(messages []types.ChatMessage) []types.ChatMessage {
	diary := strings.TrimSpace(state.GetDiaryContent())
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

func buildCalendarContext(ctx context.Context) (string, error) {
	if _, err := oauthgooglecalendar.LoadToken(); err != nil {
		return "", nil
	}
	if ctx == nil {
		return "", nil
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	token, err := oauthgooglecalendar.AccessToken(ctx)
	if err != nil {
		return "", err
	}
	now := time.Now()
	day0 := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	dayN := day0.AddDate(0, 0, calendarPromptDays)
	events, err := fetchPrimaryCalendarEvents(ctx, token, day0, dayN, calendarFetchMaxResults)
	if err != nil {
		return "", err
	}
	return formatCalendarPrompt(events, day0), nil
}
