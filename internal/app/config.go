package app

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/tetetratra/smart-speaker/internal/oauth/googlecalendar"
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
	GoogleCalendar             googlecalendar.Config
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
	DeviceMap map[string]string
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

// DefaultConfigPathは設定ファイルの既定の場所。作業ディレクトリからの相対パス。
const DefaultConfigPath = "config.yaml"

// LoadConfigはシークレットを環境変数から、それ以外の設定値をconfig.yamlから読み込む。
// config.yamlが無い場合や項目が書かれていない場合は、アプリケーション側の既定値を使う。
func LoadConfig(promptPath string) Config {
	file, err := readConfigFile(DefaultConfigPath)
	if err != nil {
		log.Fatal(err)
	}
	cfg, err := buildConfig(file, os.Getenv)
	if err != nil {
		log.Fatal(err)
	}
	cfg.SystemPrompt = readSystemPrompt(promptPath)
	cfg.STTPhrases = readSTTPhrases("stt_phrases.txt")
	return cfg
}

// configFileはconfig.yamlの構造。項目の説明はconfig.sample.yamlに書く。
// 0や空文字を有効な値として扱う項目は、未指定と区別するためにポインタにする。
type configFile struct {
	Server struct {
		WSAddr     string `yaml:"ws_addr"`
		WebDistDir string `yaml:"web_dist_dir"`
	} `yaml:"server"`
	RTC struct {
		ICEAdvertiseIPs []string `yaml:"ice_advertise_ips"`
	} `yaml:"rtc"`
	OpenAI struct {
		ResponsesModel string `yaml:"responses_model"`
		SubagentModel  string `yaml:"subagent_model"`
		Realtime       struct {
			Model              string `yaml:"model"`
			VAD                string `yaml:"vad"`
			TranscriptionModel string `yaml:"transcription_model"`
			Endpoint           string `yaml:"endpoint"`
		} `yaml:"realtime"`
	} `yaml:"openai"`
	Conversation struct {
		IdleTimeoutSeconds *int `yaml:"idle_timeout_seconds"`
	} `yaml:"conversation"`
	Memory struct {
		Model     string `yaml:"model"`
		StorePath string `yaml:"store_path"`
	} `yaml:"memory"`
	Automations struct {
		StorePath string `yaml:"store_path"`
	} `yaml:"automations"`
	STT struct {
		Provider    string `yaml:"provider"`
		OpenAIModel string `yaml:"openai_model"`
		Google      struct {
			Project    string `yaml:"project"`
			Recognizer string `yaml:"recognizer"`
			Language   string `yaml:"language"`
		} `yaml:"google"`
	} `yaml:"stt"`
	TTS struct {
		Provider string `yaml:"provider"`
		Voicevox struct {
			Endpoint   string   `yaml:"endpoint"`
			SpeakerID  *int     `yaml:"speaker_id"`
			SpeedScale *float64 `yaml:"speed_scale"`
		} `yaml:"voicevox"`
		ElevenLabs struct {
			VoiceID string `yaml:"voice_id"`
			Model   string `yaml:"model"`
		} `yaml:"elevenlabs"`
	} `yaml:"tts"`
	Wake struct {
		StorePath        string   `yaml:"store_path"`
		LocalSTTEndpoint string   `yaml:"local_stt_endpoint"`
		Words            []string `yaml:"words"`
		MaxPrefix        *int     `yaml:"max_prefix"`
		MinConfidence    *float64 `yaml:"min_confidence"`
		MinSimilarity    *float64 `yaml:"min_similarity"`
	} `yaml:"wake"`
	SwitchBot struct {
		DeviceMap map[string]string `yaml:"device_map"`
	} `yaml:"switchbot"`
	GoogleCalendar struct {
		RedirectURL string `yaml:"redirect_url"`
		Scope       string `yaml:"scope"`
		TokenPath   string `yaml:"token_path"`
	} `yaml:"google_calendar"`
}

// readConfigFileは設定ファイルを読む。ファイルが無い場合は全項目を既定値にする。
// 書き間違いに気づけるよう、未知のキーや型の誤りはエラーにする。
func readConfigFile(path string) (configFile, error) {
	var file configFile
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		log.Printf("config: %s not found; using default settings", path)
		return file, nil
	}
	if err != nil {
		return file, fmt.Errorf("failed to read %s: %w", path, err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&file); err != nil && !errors.Is(err, io.EOF) {
		return file, fmt.Errorf("failed to parse %s: %w", path, err)
	}
	return file, nil
}

// buildConfigは設定ファイルの値と環境変数のシークレットから、既定値を補ったConfigを作る。
func buildConfig(file configFile, getenv func(string) string) (Config, error) {
	env := func(name string) string { return strings.TrimSpace(getenv(name)) }

	apiKey := env("OPENAI_API_KEY")
	if apiKey == "" {
		return Config{}, errors.New("OPENAI_API_KEY is not set")
	}

	// Responsesは履歴の要約・長期メモリ・検索用に使用する。
	responsesModel := stringOr(file.OpenAI.ResponsesModel, "gpt-5.4-nano-2026-03-17")
	memoryCfg := MemoryConfig{
		Model:     stringOr(file.Memory.Model, responsesModel),
		StorePath: stringOr(file.Memory.StorePath, "data/memories.json"),
	}
	dataDir := filepath.Dir(memoryCfg.StorePath)

	// 選択肢
	// eleven_multilingual_v2: 漢字で読めないことが多い
	// eleven_flash_v2_5: 未検証。数字や日付の読み上げが難点らしい。リアルタイム向け
	// eleven_turbo_v2_5: 40,000 文字/リクエスト の上限あり
	// eleven_v3: 高品質だがレイテンシは高め。HTTPストリーミングで利用する
	elevenLabsModel := stringOr(file.TTS.ElevenLabs.Model, "eleven_v3")

	// デプロイスクリプトが本番サーバーのIPを取得して渡すため、環境変数があればそちらを優先する。
	iceIPs := trimStrings(file.RTC.ICEAdvertiseIPs)
	if raw := env("RTC_ICE_ADVERTISE_IPS"); raw != "" {
		iceIPs = splitComma(raw)
	}

	return Config{
		APIKey:                     apiKey,
		RealtimeModel:              stringOr(file.OpenAI.Realtime.Model, "gpt-realtime-2.1"),
		RealtimeVAD:                stringOr(file.OpenAI.Realtime.VAD, "semantic_vad"),
		RealtimeEndpoint:           strings.TrimSpace(file.OpenAI.Realtime.Endpoint),
		RealtimeTranscriptionModel: stringOr(file.OpenAI.Realtime.TranscriptionModel, "gpt-4o-mini-transcribe"),
		ResponsesModel:             responsesModel,
		SubagentModel:              stringOr(file.OpenAI.SubagentModel, "gpt-5.4"),
		WakeModeStorePath:          stringOr(file.Wake.StorePath, filepath.Join(dataDir, "wake-mode.json")),
		WakeSettings:               wakeSettings(file),
		LocalSTTEndpoint:           stringOr(file.Wake.LocalSTTEndpoint, "http://local-stt:8000/transcribe"),
		AutomationStorePath:        stringOr(file.Automations.StorePath, filepath.Join(dataDir, "automations.json")),
		Memory:                     memoryCfg,
		ConversationIdleTimeout:    conversationIdleTimeout(file.Conversation.IdleTimeoutSeconds),
		STTProvider:                stringOr(file.STT.Provider, "google"),
		OpenAISTTModel:             stringOr(file.STT.OpenAIModel, "gpt-realtime-whisper"),
		TTSProvider:                stringOr(file.TTS.Provider, "voicevox"),
		ElevenLabs: ElevenLabsConfig{
			APIKey:  env("ELEVENLABS_API_KEY"),
			VoiceID: strings.TrimSpace(file.TTS.ElevenLabs.VoiceID),
			Model:   elevenLabsModel,
		},
		Voicevox: VoicevoxConfig{
			Endpoint:   stringOr(file.TTS.Voicevox.Endpoint, "http://voicevox:50021"),
			SpeakerID:  intOr(file.TTS.Voicevox.SpeakerID, 1),
			SpeedScale: positiveFloat(file.TTS.Voicevox.SpeedScale),
		},
		SwitchBot: SwitchBotConfig{
			Token:     env("SWITCHBOT_TOKEN"),
			Secret:    env("SWITCHBOT_SECRET"),
			DeviceMap: file.SwitchBot.DeviceMap,
		},
		GoogleCloudProject: strings.TrimSpace(file.STT.Google.Project),
		GoogleRecognizer:   stringOr(file.STT.Google.Recognizer, "_"),
		GoogleLanguage:     stringOr(file.STT.Google.Language, "ja-JP"),
		GoogleCredentials:  env("GOOGLE_APPLICATION_CREDENTIALS_JSON"),
		GoogleCalendar: googlecalendar.Config{
			ClientID:     env("GOOGLE_CLIENT_ID"),
			ClientSecret: env("GOOGLE_CLIENT_SECRET"),
			RedirectURL:  stringOr(file.GoogleCalendar.RedirectURL, googlecalendar.DefaultRedirectURL),
			Scopes:       []string{stringOr(file.GoogleCalendar.Scope, googlecalendar.DefaultScope)},
			TokenPath:    stringOr(file.GoogleCalendar.TokenPath, filepath.Join(dataDir, "google_oauth_token.json")),
		},
		RTCIceAdvertiseIPs: iceIPs,
		WSAddr:             stringOr(file.Server.WSAddr, ":8081"),
		WebDistDir:         stringOr(file.Server.WebDistDir, "web/dist"),
	}, nil
}

// wakeSettingsは呼びかけの判定の設定を作る。値の範囲はwakemode.NewStoreで検証する。
func wakeSettings(file configFile) wakemode.Settings {
	settings := wakemode.DefaultSettings()
	if words := trimStrings(file.Wake.Words); len(words) > 0 {
		settings.Words = words
	}
	settings.MaxPrefix = intOr(file.Wake.MaxPrefix, settings.MaxPrefix)
	if file.Wake.MinConfidence != nil {
		settings.MinConfidence = *file.Wake.MinConfidence
	}
	if file.Wake.MinSimilarity != nil {
		settings.MinSimilarity = *file.Wake.MinSimilarity
	}
	return settings
}

func stringOr(value, defaultValue string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return defaultValue
}

func intOr(value *int, defaultValue int) int {
	if value == nil {
		return defaultValue
	}
	return *value
}

func positiveFloat(value *float64) *float64 {
	if value == nil || *value <= 0 {
		return nil
	}
	v := *value
	return &v
}

// conversationIdleTimeoutは会話終了までの無操作時間を返す。0は無効、負の値は既定値にする。
func conversationIdleTimeout(seconds *int) time.Duration {
	const defaultConversationIdleTimeout = 5 * time.Minute
	if seconds == nil || *seconds < 0 {
		return defaultConversationIdleTimeout
	}
	return time.Duration(*seconds) * time.Second
}

func trimStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
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
