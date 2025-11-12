package app

import (
	"fmt"
	"log"
	"os"
	"strings"
)

// Config aggregates environment-driven settings.
type Config struct {
	APIKey             string
	Model              string
	TranscriptionModel string
	InputVoicePath     string
	SystemPrompt       string
}

// LoadConfig reads environment variables and system prompt file.
func LoadConfig(promptPath string) Config {
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY is not set")
	}

	model := strings.TrimSpace(os.Getenv("OPENAI_REALTIME_MODEL"))
	if model == "" {
		model = "gpt-realtime"
	}

	var transcription string
	if value, ok := os.LookupEnv("OPENAI_TRANSCRIPTION_MODEL"); ok {
		transcription = strings.TrimSpace(value)
	} else {
		transcription = "gpt-4o-transcribe"
	}

	prompt := readSystemPrompt(promptPath)

	return Config{
		APIKey:             apiKey,
		Model:              model,
		TranscriptionModel: transcription,
		InputVoicePath:     strings.TrimSpace(os.Getenv("INPUT_VOICE")),
		SystemPrompt:       prompt,
	}
}

func readSystemPrompt(path string) string {
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to read system prompt (%v)\n", err)
		return ""
	}
	return string(data)
}
