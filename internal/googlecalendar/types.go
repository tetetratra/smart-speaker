package googlecalendar

import "time"

const (
	defaultCalendarID = "primary"
	defaultOrderBy    = "startTime"
)

type Event struct {
	ID          string
	Summary     string
	Description string
	Location    string
	HTMLLink    string
	Start       EventTime
	End         EventTime
}

type EventTime struct {
	Date     string
	DateTime string
	TimeZone string
}

type ListEventsRequest struct {
	CalendarID   string
	TimeMin      time.Time
	TimeMax      time.Time
	SingleEvents bool
	OrderBy      string
	MaxResults   int
}

type CreateEventRequest struct {
	CalendarID  string
	Summary     string
	Description string
	Location    string
	Start       EventTime
	End         EventTime
}

type UpdateEventRequest struct {
	CalendarID  string
	EventID     string
	Summary     *string
	Description *string
	Location    *string
	Start       *EventTime
	End         *EventTime
}

type DeleteEventRequest struct {
	CalendarID string
	EventID    string
}

func normalizeCalendarID(id string) string {
	if id == "" {
		return defaultCalendarID
	}
	return id
}
