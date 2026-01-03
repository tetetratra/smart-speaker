package state

var diaryContent struct {
	text string
}

// SetDiaryContent stores diary content for system prompt injection.
func SetDiaryContent(text string) {
	diaryContent.text = text
}

// GetDiaryContent returns diary content for system prompt injection.
func GetDiaryContent() string {
	return diaryContent.text
}
