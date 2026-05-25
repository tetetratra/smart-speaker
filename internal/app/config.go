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
	APIKey                  string
	ResponsesModel          string
	SystemPrompt            string
	ConversationIdleTimeout time.Duration
	AutoPromptMessage       string
	ElevenLabs              ElevenLabsConfig
	SwitchBot               SwitchBotConfig
	GoogleCloudProject      string
	GoogleRecognizer        string
	GoogleLanguage          string
	GoogleCredentials       string
	STTPhrases              []string
	RTCIceHostIPs           []string
	WSAddr                  string
	WebDistDir              string
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
		// realtime系のapiはコンテキストウィンドウが小さいのと高いため、response系のモデルを使う
		responsesModel = "gpt-5.2-2025-12-11"
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

	conversationIdleTimeout := conversationIdleTimeoutFromEnv(os.Getenv("CONVERSATION_IDLE_TIMEOUT_SECONDS"))
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
	sttPhrases := readSTTPhrases("stt_phrases.txt")

	return Config{
		APIKey:                  apiKey,
		ResponsesModel:          responsesModel,
		SystemPrompt:            prompt,
		ConversationIdleTimeout: conversationIdleTimeout,
		AutoPromptMessage:       message,
		ElevenLabs:              elv,
		SwitchBot:               switchCfg,
		GoogleCloudProject:      googleProject,
		GoogleRecognizer:        googleRecognizer,
		GoogleLanguage:          googleLanguage,
		GoogleCredentials:       googleCredentials,
		STTPhrases:              sttPhrases,
		RTCIceHostIPs:           rtcIceHostIPs,
		WSAddr:                  wsAddr,
		WebDistDir:              webDistDir,
	}
}

func conversationIdleTimeoutFromEnv(raw string) time.Duration {
	const defaultConversationIdleTimeout = 10 * time.Minute
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return defaultConversationIdleTimeout
	}
	secs, err := strconv.Atoi(trimmed)
	if err != nil || secs < 0 {
		return defaultConversationIdleTimeout
	}
	return time.Duration(secs) * time.Second
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

func readSTTPhrases(path string) []string {
	phrases, err := readPhraseFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "warning: failed to read STT phrases (%v)\n", err)
	}

	localPath := filepath.Join(filepath.Dir(path), "stt_phrases.local.txt")
	localPhrases, err := readPhraseFile(localPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "warning: failed to read local STT phrases (%v)\n", err)
	}

	return uniqueStrings(append(phrases, localPhrases...))
}

func readPhraseFile(path string) ([]string, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	phrases := make([]string, 0, len(lines))
	for _, line := range lines {
		phrase := strings.TrimSpace(line)
		if phrase == "" || strings.HasPrefix(phrase, "#") {
			continue
		}
		phrases = append(phrases, phrase)
	}
	return phrases, nil
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
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
