package realtimeapi

// OpenAI Realtime API と通信するための設定値
type Config struct {
	APIKey             string
	Model              string
	TranscriptionModel string
	Instructions       string
}
