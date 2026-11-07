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

	"github.com/tetetratra/smart-speaker/internal/states/wakemode"
)

// Config holds runtime settings.
type Config struct {
	WakeModeStorePath          string
	WakeSettings               wakemode.Settings
	LocalSTTEndpoint           string
	APIKey                     string
	ResponsesModel             string
	AutomationStorePath        string
	SubagentModel              string
	RealtimeModel              string
	RealtimeVAD                string
	RealtimeEndpoint           string
	RealtimeTranscriptionModel string
	Memory                     MemoryConfig
	SystemPrompt               string
	ConversationIdleTimeout    time.Duration
	AutoPromptMessage          string
	STTProvider                string
	OpenAISTTModel             string
	TTSProvider                string
	ElevenLabs                 ElevenLabsConfig
	Voicevox                   VoicevoxConfig
	SwitchBot                  SwitchBotConfig
	GoogleCloudProject         string
	GoogleRecognizer           string
	GoogleLanguage             string
	GoogleCredentials          string
	STTPhrases                 []string
	RTCIceAdvertiseIPs         []string
	WSAddr                     string
	WebDistDir                 string
}

type MemoryConfig struct {
	Model     string
	StorePath string
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

type VoicevoxConfig struct {
	Endpoint   string
	SpeakerID  int
	SpeedScale *float64
}

func LoadConfig(promptPath string) Config {
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY is not set")
	}
	responsesModel := strings.TrimSpace(os.Getenv("OPENAI_RESPONSES_MODEL"))
	if responsesModel == "" {
		// Responsesは履歴の要約・長期メモリ・検索用に使用する。
		responsesModel = "gpt-5.4-nano-2026-03-17"
	}
	subagentModel := strings.TrimSpace(os.Getenv("OPENAI_SUBAGENT_MODEL"))
	if subagentModel == "" {
		subagentModel = "gpt-5.4"
	}
	memoryCfg := loadMemoryConfig(responsesModel)
	wakePath := strings.TrimSpace(os.Getenv("WAKE_MODE_STORE_PATH"))
	if wakePath == "" {
		wakePath = filepath.Join(filepath.Dir(memoryCfg.StorePath), "wake-mode.json")
	}
	localSTT := strings.TrimSpace(os.Getenv("LOCAL_STT_ENDPOINT"))
	if localSTT == "" {
		localSTT = "http://localhost:8000/transcribe"
	}
	automationPath := strings.TrimSpace(os.Getenv("AUTOMATIONS_STORE_PATH"))
	if automationPath == "" {
		automationPath = filepath.Join(filepath.Dir(memoryCfg.StorePath), "automations.json")
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

	ttsProvider := strings.TrimSpace(os.Getenv("TTS_PROVIDER"))
	if ttsProvider == "" {
		ttsProvider = "voicevox"
	}
	voicevox := VoicevoxConfig{
		Endpoint:   strings.TrimSpace(os.Getenv("VOICEVOX_ENDPOINT")),
		SpeakerID:  intFromEnv("VOICEVOX_SPEAKER_ID", 1),
		SpeedScale: optionalPositiveFloatFromEnv("VOICEVOX_SPEED_SCALE"),
	}

	if voicevox.Endpoint == "" {
		voicevox.Endpoint = "http://voicevox:50021"
	}
	realtimeModel := strings.TrimSpace(os.Getenv("OPENAI_REALTIME_MODEL"))
	if realtimeModel == "" {
		realtimeModel = "gpt-realtime-2.1"
	}
	realtimeVAD := strings.TrimSpace(os.Getenv("OPENAI_REALTIME_VAD"))
	if realtimeVAD == "" {
		realtimeVAD = "semantic_vad"
	}
	transcriptionModel := strings.TrimSpace(os.Getenv("OPENAI_REALTIME_TRANSCRIPTION_MODEL"))
	if transcriptionModel == "" {
		transcriptionModel = "gpt-4o-mini-transcribe"
	}
	rtcIceAdvertiseIPs := splitComma(os.Getenv("RTC_ICE_ADVERTISE_IPS"))
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
	sttProvider := strings.TrimSpace(os.Getenv("STT_PROVIDER"))
	if sttProvider == "" {
		sttProvider = "google"
	}
	openAISTTModel := strings.TrimSpace(os.Getenv("OPENAI_STT_MODEL"))
	if openAISTTModel == "" {
		openAISTTModel = "gpt-realtime-whisper"
	}

	return Config{
		APIKey:                     apiKey,
		RealtimeModel:              realtimeModel,
		RealtimeVAD:                realtimeVAD,
		RealtimeEndpoint:           strings.TrimSpace(os.Getenv("OPENAI_REALTIME_ENDPOINT")),
		RealtimeTranscriptionModel: transcriptionModel,
		ResponsesModel:             responsesModel,
		WakeModeStorePath:          wakePath,
		WakeSettings:               loadWakeSettings(),
		LocalSTTEndpoint:           localSTT,
		AutomationStorePath:        automationPath,
		SubagentModel:              subagentModel,
		Memory:                     memoryCfg,
		SystemPrompt:               prompt,
		ConversationIdleTimeout:    conversationIdleTimeout,
		AutoPromptMessage:          message,
		STTProvider:                sttProvider,
		OpenAISTTModel:             openAISTTModel,
		TTSProvider:                ttsProvider,
		ElevenLabs:                 elv,
		Voicevox:                   voicevox,
		SwitchBot:                  switchCfg,
		GoogleCloudProject:         googleProject,
		GoogleRecognizer:           googleRecognizer,
		GoogleLanguage:             googleLanguage,
		GoogleCredentials:          googleCredentials,
		STTPhrases:                 sttPhrases,
		RTCIceAdvertiseIPs:         rtcIceAdvertiseIPs,
		WSAddr:                     wsAddr,
		WebDistDir:                 webDistDir,
	}
}

func loadMemoryConfig(responsesModel string) MemoryConfig {
	memoryModel := strings.TrimSpace(os.Getenv("OPENAI_MEMORY_MODEL"))
	if memoryModel == "" {
		memoryModel = responsesModel
	}
	storePath := strings.TrimSpace(os.Getenv("MEMORY_STORE_PATH"))
	if storePath == "" {
		storePath = "data/memories.json"
	}
	return MemoryConfig{
		Model:     memoryModel,
		StorePath: storePath,
	}
}

// loadWakeSettingsは呼びかけの判定の設定を環境変数から読む。値の範囲はwakemode.NewStoreで検証する。
func loadWakeSettings() wakemode.Settings {
	settings := wakemode.DefaultSettings()
	if words := strings.TrimSpace(os.Getenv("WAKE_WORDS")); words != "" {
		settings.Words = nil
		for _, word := range strings.Split(words, ",") {
			if word = strings.TrimSpace(word); word != "" {
				settings.Words = append(settings.Words, word)
			}
		}
	}
	settings.MaxPrefix = intFromEnv("WAKE_MAX_PREFIX", settings.MaxPrefix)
	settings.MinConfidence = floatFromEnv("WAKE_MIN_CONFIDENCE", settings.MinConfidence)
	settings.MinSimilarity = floatFromEnv("WAKE_MIN_SIMILARITY", settings.MinSimilarity)
	return settings
}

func intFromEnv(name string, defaultValue int) int {
	trimmed := strings.TrimSpace(os.Getenv(name))
	if trimmed == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(trimmed)
	if err != nil {
		return defaultValue
	}
	return value
}

func floatFromEnv(name string, defaultValue float64) float64 {
	trimmed := strings.TrimSpace(os.Getenv(name))
	if trimmed == "" {
		return defaultValue
	}
	value, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return defaultValue
	}
	return value
}

func optionalPositiveFloatFromEnv(name string) *float64 {
	trimmed := strings.TrimSpace(os.Getenv(name))
	if trimmed == "" {
		return nil
	}
	value, err := strconv.ParseFloat(trimmed, 64)
	if err != nil || value <= 0 {
		return nil
	}
	return &value
}

func conversationIdleTimeoutFromEnv(raw string) time.Duration {
	const defaultConversationIdleTimeout = 5 * time.Minute
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
