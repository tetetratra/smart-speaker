package types

import "encoding/json"

const (
	TimelineKindSpeech = "speech"
	TimelineKindWait   = "wait"
	TimelineKindTool   = "tool"
)

// TimelineItem は LLM が出力する順序付き item です。
type TimelineItem struct {
	Kind         string
	GenerationID GenerationID
	SequenceID   string
	Text         string
	Sec          float64
	ToolName     string
	ToolArgs     json.RawMessage
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
