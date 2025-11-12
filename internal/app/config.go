package app

import (
	"fmt"
	"log"
	"os"
	"strings"
)

// 環境変数などから集めた設定値を保持する
type Config struct {
	APIKey             string
	Model              string
	TranscriptionModel string
	InputVoicePath     string
	SystemPrompt       string
}

// 環境変数とシステムプロンプトファイルを読み込む
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

	return Config{
		APIKey:             apiKey,
		Model:              model,
		TranscriptionModel: transcription,
		InputVoicePath:     inputVoicePath,
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
