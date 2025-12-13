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
	Voice              string
	SwitchBot          SwitchBotConfig
	Debug              DebugConfig
	WSAddr             string
}

type SwitchBotConfig struct {
	Token     string
	Secret    string
	DeviceMap string
}

type DebugConfig struct {
	PrintMsgType  bool
	DumpResponses bool
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

	voice := strings.TrimSpace(os.Getenv("OPENAI_VOICE"))
	if voice == "" {
		voice = "marin"
	}

	transcription := strings.TrimSpace(os.Getenv("OPENAI_TRANSCRIPTION_MODEL"))
	inputVoicePath := strings.TrimSpace(os.Getenv("INPUT_VOICE"))
	prompt := readSystemPrompt(promptPath)

	wsAddr := strings.TrimSpace(os.Getenv("WS_ADDR"))
	if wsAddr == "" {
		wsAddr = ":8081"
	}

	interval := time.Minute * 10
	if raw := strings.TrimSpace(os.Getenv("AUTO_PROMPT_INTERVAL")); raw != "" {
		if secs, err := strconv.Atoi(raw); err == nil && secs > 0 {
			interval = time.Duration(secs) * time.Second
		}
	}
	message := strings.TrimSpace(os.Getenv("AUTO_PROMPT_MESSAGE"))

	switchCfg := SwitchBotConfig{
		Token:     os.Getenv("SWITCHBOT_TOKEN"),
		Secret:    os.Getenv("SWITCHBOT_SECRET"),
		DeviceMap: os.Getenv("SWITCHBOT_DEVICE_MAP"),
	}

	return Config{
		APIKey:             apiKey,
		Model:              model,
		TranscriptionModel: transcription,
		InputVoicePath:     inputVoicePath,
		SystemPrompt:       prompt,
		AutoPromptInterval: interval,
		AutoPromptMessage:  message,
		Voice:              voice,
		SwitchBot:          switchCfg,
		WSAddr:             wsAddr,
		Debug: DebugConfig{
			PrintMsgType:  envBool("SMART_SPEAKER_DEBUG_PRINT_MSG_TYPE"),
			DumpResponses: envBool("SMART_SPEAKER_DEBUG_DUMP_RESPONSES"),
		},
	}
}

func envBool(name string) bool {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return false
	}
	switch strings.ToLower(v) {
	case "1", "true", "t", "yes", "y", "on":
		return true
	default:
		return false
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
