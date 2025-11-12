package realtimeapi

// Config contains the options required to talk to the OpenAI Realtime API.
type Config struct {
	APIKey             string
	Model              string
	TranscriptionModel string
	Instructions       string
}
