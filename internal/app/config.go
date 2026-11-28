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

	"github.com/tetetratra/smart-speaker/internal/features/memory"
	"github.com/tetetratra/smart-speaker/internal/features/wakemode"
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
	ConversationHistoryPath    string
	RealtimeModel              string
	RealtimeVAD                string
	RealtimeNoiseReduction     string
	RealtimeEndpoint           string
	RealtimeTranscriptionModel string
	// DecisionsModel は、発話の振り分けに使う Decisions API のモデル。DecisionsEndpoint はテスト用の接続先。
	DecisionsModel    string
	DecisionsEndpoint string
	Brain             BrainConfig
	Memory            MemoryConfig
	// SystemPrompt は Realtime（ユーザーと話す音声のモデル）へ、BrainSystemPrompt は頭脳AIへ渡すシステムプロンプト。
	SystemPrompt            string
	BrainSystemPrompt       string
	ConversationIdleTimeout time.Duration
	TTSProvider             string
	ElevenLabs              ElevenLabsConfig
	Voicevox                VoicevoxConfig
	RTCIceAdvertiseIPs      []string
	WSAddr                  string
	WebDistDir              string
	// pluginConfigs は config.yaml の plugins の下の、プラグインの名前ごとの設定。中身はプラグインが読む。
	pluginConfigs map[string]yaml.Node
}

// BrainConfig は頭脳AIの設定。
type BrainConfig struct {
	Model string
	// ReasoningEffort は Responses API の reasoning.effort。
	ReasoningEffort string
	// Endpoint はテスト用の Responses API の接続先。空なら OpenAI の公式エンドポイント。
	Endpoint string
}

// DecodePluginConfig は、config.yaml の plugins.<name> の中身を v に読み込む。
// 書かれていなければ v を変えずに nil を返す。書き間違いに気づけるよう、未知のキーや型の誤りはエラーにする。
func (c Config) DecodePluginConfig(name string, v any) error {
	node, ok := c.pluginConfigs[name]
	if !ok {
		return nil
	}
	data, err := yaml.Marshal(&node)
	if err != nil {
		return fmt.Errorf("config: plugins.%s: %w", name, err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(v); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("config: plugins.%s: %w", name, err)
	}
	return nil
}

// PluginConfigNames は、config.yaml の plugins の下に書かれた名前を名前順に返す。
func (c Config) PluginConfigNames() []string {
	names := make([]string, 0, len(c.pluginConfigs))
	for name := range c.pluginConfigs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

type MemoryConfig struct {
	Model     string
	StorePath string
	// MemorizeAfter は、会話を長期メモリにするまでの経過時間。
	MemorizeAfter time.Duration
	// HistoryMaxBytes は、経過時間によらず古い会話から長期メモリにし始める履歴の大きさ。
	HistoryMaxBytes int
}

type ElevenLabsConfig struct {
	APIKey  string
	VoiceID string
	Model   string
}

type VoicevoxConfig struct {
	Endpoint    string
	SpeakerID   int
	SpeakerName string
	SpeedScale  *float64
}

// DefaultConfigPathは設定ファイルの既定の場所。作業ディレクトリからの相対パス。
const DefaultConfigPath = "config.yaml"

// ConfigPathEnvは設定ファイルの場所を変えるための環境変数。検証用の設定を読ませるときなどに使う。
const ConfigPathEnv = "SMART_SPEAKER_CONFIG"

// legacyEnvVarsは、config.yamlへ移したため読まなくなった環境変数と、移行先の項目。
// 設定したまま移行を忘れると既定値で動いてしまうため、起動時に警告する。
var legacyEnvVars = map[string]string{
	"OPENAI_RESPONSES_MODEL":              "openai.responses_model",
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
	"GOOGLE_REDIRECT_URL":                 "plugins.googlecalendar.redirect_url",
	"GOOGLE_OAUTH_SCOPE":                  "plugins.googlecalendar.scope",
	"GOOGLE_OAUTH_TOKEN_PATH":             "plugins.googlecalendar.token_path",
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
		Realtime       struct {
			Model              string `yaml:"model"`
			VAD                string `yaml:"vad"`
			NoiseReduction     string `yaml:"noise_reduction"`
			TranscriptionModel string `yaml:"transcription_model"`
			Endpoint           string `yaml:"endpoint"`
		} `yaml:"realtime"`
		Decisions struct {
			Model    string `yaml:"model"`
			Endpoint string `yaml:"endpoint"`
		} `yaml:"decisions"`
		Brain struct {
			Model           string `yaml:"model"`
			ReasoningEffort string `yaml:"reasoning_effort"`
			Endpoint        string `yaml:"endpoint"`
		} `yaml:"brain"`
	} `yaml:"openai"`
	Conversation struct {
		IdleTimeoutSeconds *int `yaml:"idle_timeout_seconds"`
	} `yaml:"conversation"`
	Memory struct {
		Model              string `yaml:"model"`
		StorePath          string `yaml:"store_path"`
		MemorizeAfterHours *int   `yaml:"memorize_after_hours"`
		HistoryMaxBytes    *int   `yaml:"history_max_bytes"`
	} `yaml:"memory"`
	Automations struct {
		StorePath string `yaml:"store_path"`
	} `yaml:"automations"`
	TTS struct {
		Provider string `yaml:"provider"`
		Voicevox struct {
			Endpoint    string   `yaml:"endpoint"`
			SpeakerID   *int     `yaml:"speaker_id"`
			SpeakerName string   `yaml:"speaker_name"`
			SpeedScale  *float64 `yaml:"speed_scale"`
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
	// Plugins はプラグインの名前ごとの設定。中身はプラグインが自分で読む（Config.DecodePluginConfig）。
	Plugins map[string]yaml.Node `yaml:"plugins"`
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

	// Responsesは履歴の要約・検索用に使用する。
	responsesModel := stringOr(file.OpenAI.ResponsesModel, "gpt-5.4-nano-2026-03-17")
	memoryCfg := MemoryConfig{
		// 長期メモリの抽出は、古くなった記憶の削除で日付を比べる。gpt-5.4-nano では判断が安定しなかったため、別の既定値にする。
		Model:     stringOr(file.Memory.Model, "gpt-5.6-luna"),
		StorePath: stringOr(file.Memory.StorePath, "data/memories.json"),
		// 0 以下は、メモリ化の側で既定値にする。
		MemorizeAfter:   time.Duration(intOr(file.Memory.MemorizeAfterHours, int(memory.DefaultMemorizeAfter/time.Hour))) * time.Hour,
		HistoryMaxBytes: intOr(file.Memory.HistoryMaxBytes, memory.DefaultMaxHistoryBytes),
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
		BrainSystemPrompt:          joinPrompts(defaultBrainPrompt, file.AdditionalSystemPrompt),
		RealtimeModel:              stringOr(file.OpenAI.Realtime.Model, "gpt-realtime-2.1"),
		RealtimeVAD:                stringOr(file.OpenAI.Realtime.VAD, "semantic_vad"),
		RealtimeNoiseReduction:     stringOr(file.OpenAI.Realtime.NoiseReduction, "far_field"),
		RealtimeEndpoint:           strings.TrimSpace(file.OpenAI.Realtime.Endpoint),
		RealtimeTranscriptionModel: stringOr(file.OpenAI.Realtime.TranscriptionModel, "gpt-4o-mini-transcribe"),
		ResponsesModel:             responsesModel,
		DecisionsModel:             stringOr(file.OpenAI.Decisions.Model, "gpt-6-luna"),
		DecisionsEndpoint:          strings.TrimSpace(file.OpenAI.Decisions.Endpoint),
		Brain: BrainConfig{
			Endpoint:        strings.TrimSpace(file.OpenAI.Brain.Endpoint),
			Model:           stringOr(file.OpenAI.Brain.Model, "gpt-6.1-terra"),
			ReasoningEffort: stringOr(file.OpenAI.Brain.ReasoningEffort, "low"),
		},
		WakeModeStorePath:       stringOr(file.Wake.StorePath, filepath.Join(dataDir, "wake-mode.json")),
		WakeSettings:            wake,
		LocalSTTEndpoint:        stringOr(file.Wake.LocalSTTEndpoint, "http://local-stt:8000/transcribe"),
		WakeEnergyThreshold:     energyThreshold,
		AutomationStorePath:     stringOr(file.Automations.StorePath, filepath.Join(dataDir, "automations.json")),
		ConversationHistoryPath: filepath.Join(dataDir, "conversation-history.json"),
		Memory:                  memoryCfg,
		ConversationIdleTimeout: idleTimeout,
		TTSProvider:             stringOr(file.TTS.Provider, "voicevox"),
		ElevenLabs: ElevenLabsConfig{
			APIKey:  env("ELEVENLABS_API_KEY"),
			VoiceID: strings.TrimSpace(file.TTS.ElevenLabs.VoiceID),
			Model:   elevenLabsModel,
		},
		Voicevox: VoicevoxConfig{
			Endpoint:    stringOr(file.TTS.Voicevox.Endpoint, "http://voicevox:50021"),
			SpeakerID:   intOr(file.TTS.Voicevox.SpeakerID, 1),
			SpeakerName: voicevoxSpeakerName(file.TTS.Voicevox.SpeakerName, file.TTS.Voicevox.SpeakerID),
			SpeedScale:  positiveFloat(file.TTS.Voicevox.SpeedScale),
		},
		RTCIceAdvertiseIPs: iceIPs,
		WSAddr:             stringOr(file.Server.WSAddr, ":8081"),
		WebDistDir:         stringOr(file.Server.WebDistDir, "web/dist"),
		pluginConfigs:      file.Plugins,
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

// conversationIdleTimeoutは古い会話履歴を長期メモリ化するまでの無操作時間を返す。0は無効、負の値は既定値にする。
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

// voicevoxSpeakerNameは、クレジット表記に使う話者名を決める。
// 既定のスタイルID 1 はずんだもんなので、speaker_id も省略されたときだけ「ずんだもん」を補う。
// speaker_id だけ変えた場合に別の話者名を表示しないよう、そのときは空のままにする。
func voicevoxSpeakerName(name string, speakerID *int) string {
	name = strings.TrimSpace(name)
	if name == "" && speakerID == nil {
		return "ずんだもん"
	}
	return name
}
