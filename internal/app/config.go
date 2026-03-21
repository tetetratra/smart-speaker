package app

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
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
	GoogleCloudProject string
	GoogleRecognizer   string
	GoogleLanguage     string
	GoogleCredentials  string
	RTCIceHostIPs      []string
	WSAddr             string
	WebDistDir         string
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

func LoadConfig(promptPath string) Config {
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY is not set")
	}
	responsesModel := strings.TrimSpace(os.Getenv("OPENAI_RESPONSES_MODEL"))
	if responsesModel == "" {
		responsesModel = "gpt-5.3-chat-latest"
	}

	prompt := readSystemPrompt(promptPath)

	wsAddr := strings.TrimSpace(os.Getenv("WS_ADDR"))
	if wsAddr == "" {
		wsAddr = ":8081"
	}
	webDistDir := strings.TrimSpace(os.Getenv("WEB_DIST_DIR"))
	if webDistDir == "" {
		webDistDir = "web/dist"
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
		// 選択肢
		// eleven_multilingual_v2: 漢字で読めないことが多い
		// eleven_flash_v2_5: 未検証。数字や日付の読み上げが難点らしい。リアルタイム向け
		// eleven_turbo_v2_5: 40,000 文字/リクエスト の上限あり
		// eleven_v3: 高品質だがレイテンシは高め。HTTPストリーミングで利用する
		elv.Model = "eleven_v3"
	}

	rtcIceHostIPs := splitComma(os.Getenv("RTC_ICE_HOST_IPS"))
	googleProject := strings.TrimSpace(os.Getenv("GOOGLE_CLOUD_PROJECT"))
	googleRecognizer := strings.TrimSpace(os.Getenv("GOOGLE_SPEECH_RECOGNIZER"))
	if googleRecognizer == "" {
		googleRecognizer = "_"
	}
	googleLanguage := strings.TrimSpace(os.Getenv("GOOGLE_SPEECH_LANGUAGE"))
	if googleLanguage == "" {
		googleLanguage = "ja-JP"
	}
	googleCredentials := strings.TrimSpace(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS_JSON"))

	return Config{
		APIKey:             apiKey,
		ResponsesModel:     responsesModel,
		SystemPrompt:       prompt,
		AutoPromptInterval: interval,
		AutoPromptMessage:  message,
		ElevenLabs:         elv,
		SwitchBot:          switchCfg,
		GoogleCloudProject: googleProject,
		GoogleRecognizer:   googleRecognizer,
		GoogleLanguage:     googleLanguage,
		GoogleCredentials:  googleCredentials,
		RTCIceHostIPs:      rtcIceHostIPs,
		WSAddr:             wsAddr,
		WebDistDir:         webDistDir,
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

func readSystemPrompt(path string) string {
	if path == "" {
		return ""
	}
	mainPrompt, err := readPromptFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to read system prompt (%v)\n", err)
	}

	localPath := filepath.Join(filepath.Dir(path), "system_prompt.local.txt")
	localPrompt, err := readPromptFile(localPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "warning: failed to read local system prompt (%v)\n", err)
	}

	return joinPrompts(mainPrompt, localPrompt)
}

func readPromptFile(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func joinPrompts(mainPrompt, localPrompt string) string {
	if strings.TrimSpace(mainPrompt) == "" {
		return localPrompt
	}
	if strings.TrimSpace(localPrompt) == "" {
		return mainPrompt
	}
	mainPrompt = strings.TrimRight(mainPrompt, "\n")
	localPrompt = strings.TrimLeft(localPrompt, "\n")
	return mainPrompt + "\n\n" + localPrompt
}
