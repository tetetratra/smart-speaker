package app

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/tetetratra/smart-speaker/internal/oauth/googlecalendar"
	"github.com/tetetratra/smart-speaker/internal/states/wakemode"
)

// Config holds runtime settings.
type Config struct {
	WakeModeStorePath string
	WakeSettings      wakemode.Settings
	LocalSTTEndpoint  string
	// WakeEnergyThresholdは、呼びかけ必須モードで発話とみなすマイクの音量（RMS、0より大きく1以下）。
	WakeEnergyThreshold        float64
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
	TTSProvider                string
	ElevenLabs                 ElevenLabsConfig
	Voicevox                   VoicevoxConfig
	SwitchBot                  SwitchBotConfig
	GoogleCalendar             googlecalendar.Config
	RTCIceAdvertiseIPs         []string
	WSAddr                     string
	WebDistDir                 string
	AgentBridge                AgentBridgeConfig
}

// AgentBridgeConfig は手元のMacで動く agent-bridge への接続設定。どちらかが空ならClaude Code連携を無効にする。
type AgentBridgeConfig struct {
	URL   string
	Token string
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

// ConfigPathEnvは設定ファイルの場所を変えるための環境変数。検証用の設定を読ませるときなどに使う。
const ConfigPathEnv = "SMART_SPEAKER_CONFIG"

// legacyEnvVarsは、config.yamlへ移したため読まなくなった環境変数と、移行先の項目。
// 設定したまま移行を忘れると既定値で動いてしまうため、起動時に警告する。
var legacyEnvVars = map[string]string{
	"OPENAI_RESPONSES_MODEL":              "openai.responses_model",
	"OPENAI_SUBAGENT_MODEL":               "openai.subagent_model",
	"OPENAI_REALTIME_MODEL":               "openai.realtime.model",
	"OPENAI_REALTIME_VAD":                 "openai.realtime.vad",
	"OPENAI_REALTIME_TRANSCRIPTION_MODEL": "openai.realtime.transcription_model",
	"OPENAI_REALTIME_ENDPOINT":            "openai.realtime.endpoint",
	"OPENAI_MEMORY_MODEL":                 "memory.model",
	"MEMORY_STORE_PATH":                   "memory.store_path",
	"AUTOMATIONS_STORE_PATH":              "automations.store_path",
	"CONVERSATION_IDLE_TIMEOUT_SECONDS":   "conversation.idle_timeout_seconds",
	"TTS_PROVIDER":                        "tts.provider",
	"VOICEVOX_ENDPOINT":                   "tts.voicevox.endpoint",
	"VOICEVOX_SPEAKER_ID":                 "tts.voicevox.speaker_id",
	"VOICEVOX_SPEED_SCALE":                "tts.voicevox.speed_scale",
	"ELEVENLABS_VOICE_ID":                 "tts.elevenlabs.voice_id",
	"ELEVENLABS_MODEL_ID":                 "tts.elevenlabs.model",
	"WAKE_MODE_STORE_PATH":                "wake.store_path",
	"LOCAL_STT_ENDPOINT":                  "wake.local_stt_endpoint",
	"WAKE_WORDS":                          "wake.words",
	"WAKE_MAX_PREFIX":                     "wake.max_prefix",
	"WAKE_MIN_CONFIDENCE":                 "wake.min_confidence",
	"WAKE_MIN_SIMILARITY":                 "wake.min_similarity",
	"SWITCHBOT_DEVICE_MAP":                "switchbot.device_map",
	"GOOGLE_REDIRECT_URL":                 "google_calendar.redirect_url",
	"GOOGLE_OAUTH_SCOPE":                  "google_calendar.scope",
	"GOOGLE_OAUTH_TOKEN_PATH":             "google_calendar.token_path",
	"WS_ADDR":                             "server.ws_addr",
	"WEB_DIST_DIR":                        "server.web_dist_dir",
}

// LoadConfigはシークレットを環境変数から、それ以外の設定値をconfig.yamlから読み込む。
// config.yamlが無い場合や項目が書かれていない場合は、アプリケーション側の既定値を使う。
func LoadConfig() Config {
	path := DefaultConfigPath
	if p := strings.TrimSpace(os.Getenv(ConfigPathEnv)); p != "" {
		path = p
	}
	file, err := readConfigFile(path)
	if err != nil {
		log.Fatal(err)
	}
	for _, warning := range legacyEnvWarnings(os.Getenv) {
		log.Print(warning)
	}
	cfg, err := buildConfig(file, os.Getenv)
	if err != nil {
		log.Fatal(err)
	}
	return cfg
}

// legacyEnvWarningsは、設定されている旧環境変数ごとの警告文を名前順に返す。
func legacyEnvWarnings(getenv func(string) string) []string {
	names := make([]string, 0, len(legacyEnvVars))
	for name := range legacyEnvVars {
		if strings.TrimSpace(getenv(name)) != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	warnings := make([]string, 0, len(names))
	for _, name := range names {
		warnings = append(warnings, fmt.Sprintf("config: warning: %s is no longer read; set %s in config.yaml instead", name, legacyEnvVars[name]))
	}
	return warnings
}

// configFileはconfig.yamlの構造。項目の説明はconfig.sample.yamlに書く。
// 0や空文字を有効な値として扱う項目は、未指定と区別するためにポインタにする。
type configFile struct {
	AdditionalSystemPrompt string `yaml:"additional_system_prompt"`
	Server                 struct {
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
		// EnergyThresholdは、発話とみなすマイクの音量（RMS）。小さい声も拾うには下げる。
		EnergyThreshold *float64 `yaml:"energy_threshold"`
		// ConversationTimeoutSecondsは、呼びかけ後に呼びかけなしで話せる時間（秒）。
		ConversationTimeoutSeconds *int `yaml:"conversation_timeout_seconds"`
	} `yaml:"wake"`
	SwitchBot struct {
		DeviceMap map[string]string `yaml:"device_map"`
	} `yaml:"switchbot"`
	GoogleCalendar struct {
		RedirectURL string `yaml:"redirect_url"`
		Scope       string `yaml:"scope"`
		TokenPath   string `yaml:"token_path"`
	} `yaml:"google_calendar"`
	AgentBridge struct {
		URL string `yaml:"url"`
	} `yaml:"agent_bridge"`
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

	idleTimeout := conversationIdleTimeout(file.Conversation.IdleTimeoutSeconds)
	wake := wakeSettings(file)
	// 無操作リセットで会話が終わった後も呼びかけなしで話せると、文脈のない会話が始まってしまうため、
	// 呼びかけなしで話せる時間は無操作リセットより短くする。無操作リセットが無効（0）なら比べない。
	if idleTimeout > 0 && wake.ConversationTimeout >= idleTimeout {
		return Config{}, fmt.Errorf("wake.conversation_timeout_seconds (%s) must be shorter than conversation.idle_timeout_seconds (%s)", wake.ConversationTimeout, idleTimeout)
	}

	energyThreshold := DefaultWakeEnergyThreshold
	if v := file.Wake.EnergyThreshold; v != nil {
		if *v <= 0 || *v > 1 {
			return Config{}, fmt.Errorf("wake.energy_threshold (%g) must be greater than 0 and at most 1", *v)
		}
		energyThreshold = *v
	}

	return Config{
		APIKey:                     apiKey,
		SystemPrompt:               joinPrompts(defaultSystemPrompt, file.AdditionalSystemPrompt),
		RealtimeModel:              stringOr(file.OpenAI.Realtime.Model, "gpt-realtime-2.1"),
		RealtimeVAD:                stringOr(file.OpenAI.Realtime.VAD, "semantic_vad"),
		RealtimeEndpoint:           strings.TrimSpace(file.OpenAI.Realtime.Endpoint),
		RealtimeTranscriptionModel: stringOr(file.OpenAI.Realtime.TranscriptionModel, "gpt-4o-mini-transcribe"),
		ResponsesModel:             responsesModel,
		SubagentModel:              stringOr(file.OpenAI.SubagentModel, "gpt-5.4"),
		WakeModeStorePath:          stringOr(file.Wake.StorePath, filepath.Join(dataDir, "wake-mode.json")),
		WakeSettings:               wake,
		LocalSTTEndpoint:           stringOr(file.Wake.LocalSTTEndpoint, "http://local-stt:8000/transcribe"),
		WakeEnergyThreshold:        energyThreshold,
		AutomationStorePath:        stringOr(file.Automations.StorePath, filepath.Join(dataDir, "automations.json")),
		Memory:                     memoryCfg,
		ConversationIdleTimeout:    idleTimeout,
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
		AgentBridge: AgentBridgeConfig{
			URL:   strings.TrimSpace(file.AgentBridge.URL),
			Token: env("AGENT_BRIDGE_TOKEN"),
		},
	}, nil
}

// DefaultWakeEnergyThresholdは、呼びかけ必須モードで発話とみなす音量（RMS）の既定値。
// 小さい声の「起きて」を取りこぼしていたため、当初の0.006から下げた。
const DefaultWakeEnergyThreshold = 0.005

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
	if file.Wake.ConversationTimeoutSeconds != nil {
		settings.ConversationTimeout = time.Duration(*file.Wake.ConversationTimeoutSeconds) * time.Second
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
	const defaultConversationIdleTimeout = 30 * time.Minute
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
