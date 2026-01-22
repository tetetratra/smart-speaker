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
	ResponsesModel     string
	SystemPrompt       string
	AutoPromptInterval time.Duration
	AutoPromptMessage  string
	ElevenLabs         ElevenLabsConfig
	SwitchBot          SwitchBotConfig
	Vosk               VoskConfig
	RTCIceHostIPs      []string
	RTCIcePortMin      int
	RTCIcePortMax      int
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

type VoskConfig struct {
	ModelPath string
}

func LoadConfig(promptPath string) Config {
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY is not set")
	}
	responsesModel := strings.TrimSpace(os.Getenv("OPENAI_RESPONSES_MODEL"))
	if responsesModel == "" {
		responsesModel = "gpt-5-chat-latest"
	}

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

	rtcIceHostIPs := splitComma(os.Getenv("RTC_ICE_HOST_IPS"))
	rtcIcePortMin := readEnvInt("RTC_ICE_PORT_MIN")
	rtcIcePortMax := readEnvInt("RTC_ICE_PORT_MAX")

	return Config{
		APIKey:             apiKey,
		ResponsesModel:     responsesModel,
		SystemPrompt:       prompt,
		AutoPromptInterval: interval,
		AutoPromptMessage:  message,
		ElevenLabs:         elv,
		SwitchBot:          switchCfg,
		Vosk: VoskConfig{
			ModelPath: strings.TrimSpace(os.Getenv("VOSK_MODEL_PATH")),
		},
		RTCIceHostIPs: rtcIceHostIPs,
		RTCIcePortMin: rtcIcePortMin,
		RTCIcePortMax: rtcIcePortMax,
		WSAddr:        wsAddr,
	}
}

func splitComma(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func readEnvInt(key string) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return 0
	}
	val, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return val
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
