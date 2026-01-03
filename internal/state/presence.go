package state

import "time"

var presence struct {
	present    bool
	capturedAt time.Time
}

// Set updates the latest presence status and capture time.
func Set(present bool, capturedAt time.Time) {
	presence.present = present
	presence.capturedAt = capturedAt
}

// Get returns the latest presence status and its capture time.
func Get() (present bool, capturedAt time.Time) {
	return presence.present, presence.capturedAt
}
