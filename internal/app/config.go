package app

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds runtime settings.
type Config struct {
	APIKey             string
	Model              string
	TranscriptionModel string
	InputVoicePath     string
	SystemPrompt       string
	AutoPromptInterval time.Duration
	AutoPromptMessage  string
}

func LoadConfig(promptPath string) Config {
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY is not set")
	}

	model := strings.TrimSpace(os.Getenv("OPENAI_REALTIME_MODEL"))
	if model == "" {
		model = "gpt-realtime"
	}

	transcription := strings.TrimSpace(os.Getenv("OPENAI_TRANSCRIPTION_MODEL"))
	inputVoicePath := strings.TrimSpace(os.Getenv("INPUT_VOICE"))
	prompt := readSystemPrompt(promptPath)

	interval := time.Minute * 10
	if raw := strings.TrimSpace(os.Getenv("AUTO_PROMPT_INTERVAL")); raw != "" {
		if secs, err := strconv.Atoi(raw); err == nil && secs > 0 {
			interval = time.Duration(secs) * time.Second
		}
	}
	message := strings.TrimSpace(os.Getenv("AUTO_PROMPT_MESSAGE"))
	if message == "" {
		message = "(system: ユーザーに状況を尋ねてください)"
	}

	return Config{
		APIKey:             apiKey,
		Model:              model,
		TranscriptionModel: transcription,
		InputVoicePath:     inputVoicePath,
		SystemPrompt:       prompt,
		AutoPromptInterval: interval,
		AutoPromptMessage:  message,
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
