package types

const TimelineKindSpeech = "speech"

// TimelineItem は speechsegment が応答テキストから作る読み上げ単位です。
type TimelineItem struct {
	ItemID       string
	Kind         string
	GenerationID GenerationID
	SequenceID   string
	Text         string
}

// PlayableSpeech は TTS 済みの speech item です。
type PlayableSpeech struct {
	GenerationID     GenerationID
	SequenceID       string
	Text             string
	Audio            string
	DurationSeconds  float64
	OriginalTimeline TimelineItem
}
