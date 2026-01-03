package state

import "time"

var diaryStatus struct {
	writtenAt time.Time
}

// SetDiaryWrittenAt marks diary as written at the provided time.
func SetDiaryWrittenAt(t time.Time) {
	diaryStatus.writtenAt = t
}

// IsDiaryWrittenSince returns true when diary was written after last activity.
func IsDiaryWrittenSince(lastActivity time.Time) bool {
	if lastActivity.IsZero() {
		return false
	}
	if diaryStatus.writtenAt.IsZero() {
		return false
	}
	return diaryStatus.writtenAt.After(lastActivity)
}
