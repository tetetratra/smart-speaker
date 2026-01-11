package app

import (
	"fmt"
	"log"
	"os"
	"smart-speaker/internal/state"
	"strconv"
	"strings"
	"time"
)

// Config holds runtime settings.
type Config struct {
	APIKey             string
	Model              string
	ResponsesModel     string
	TranscriptionModel string
	InputVoicePath     string
	SystemPrompt       string
	AutoPromptInterval time.Duration
	AutoPromptMessage  string
	Voice              string
	ElevenLabs         ElevenLabsConfig
	SwitchBot          SwitchBotConfig
	Debug              DebugConfig
	WSAddr             string
}

type SwitchBotConfig struct {
	Token     string
	Secret    string
	DeviceMap string
}

type ElevenLabsConfig struct {
	APIKey  string
	VoiceID string
	Model   string
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
	model := "gpt-realtime"
	responsesModel := strings.TrimSpace(os.Getenv("OPENAI_RESPONSES_MODEL"))
	if responsesModel == "" {
		responsesModel = "gpt-5-chat-latest"
	}
	voice := strings.TrimSpace(os.Getenv("OPENAI_VOICE"))
	if voice == "" {
		voice = "marin"
	}

	transcription := "gpt-4o-mini-transcribe"
	inputVoicePath := strings.TrimSpace(os.Getenv("INPUT_VOICE"))
	prompt := readSystemPrompt(promptPath)
	if diary := strings.TrimSpace(state.GetDiaryContent()); diary != "" {
		if strings.TrimSpace(prompt) != "" {
			prompt = strings.TrimRight(prompt, "\n") + "\n\n"
		}
		prompt = prompt + "以下は過去の会話をまとめた日記です。参考として扱ってください。\n" + diary
	}

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

	elv := ElevenLabsConfig{
		APIKey:  strings.TrimSpace(os.Getenv("ELEVENLABS_API_KEY")),
		VoiceID: strings.TrimSpace(os.Getenv("ELEVENLABS_VOICE_ID")),
		Model:   strings.TrimSpace(os.Getenv("ELEVENLABS_MODEL_ID")),
	}
	if elv.Model == "" {
		elv.Model = "eleven_multilingual_v2"
	}

	return Config{
		APIKey:             apiKey,
		Model:              model,
		ResponsesModel:     responsesModel,
		TranscriptionModel: transcription,
		InputVoicePath:     inputVoicePath,
		SystemPrompt:       prompt,
		AutoPromptInterval: interval,
		AutoPromptMessage:  message,
		Voice:              voice,
		// モダリティはデフォルトで text のみ。変更したい場合はコード側で書き換える。
		ElevenLabs: elv,
		SwitchBot:  switchCfg,
		WSAddr:     wsAddr,
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
