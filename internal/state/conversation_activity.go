package state

import "time"

var conversationActivity struct {
	lastActivityAt time.Time
}

// SetLastActivityAt updates the last conversation activity time.
func SetLastActivityAt(t time.Time) {
	conversationActivity.lastActivityAt = t
}

// GetLastActivityAt returns the last conversation activity time.
func GetLastActivityAt() time.Time {
	return conversationActivity.lastActivityAt
}
