package state

import "time"

var conversation struct {
	lastAssistantTalkAt time.Time
}

// SetLastAssistantTalkAt updates the last assistant speech time.
func SetLastAssistantTalkAt(t time.Time) {
	conversation.lastAssistantTalkAt = t
}

// GetLastAssistantTalkAt returns the last assistant speech time.
func GetLastAssistantTalkAt() time.Time {
	return conversation.lastAssistantTalkAt
}
