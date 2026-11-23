package app

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
)

// loadFromYAMLはYAML文字列と環境変数からConfigを作る。
func loadFromYAML(t *testing.T, content string, env map[string]string) (Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	file, err := readConfigFile(path)
	if err != nil {
		return Config{}, err
	}
	return buildConfig(file, func(name string) string { return env[name] })
}

func mustLoadFromYAML(t *testing.T, content string, env map[string]string) Config {
	t.Helper()
	cfg, err := loadFromYAML(t, content, env)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

var minimalEnv = map[string]string{"OPENAI_API_KEY": "openai"}

func TestBuildConfigRequiresOpenAIAPIKey(t *testing.T) {
	if _, err := loadFromYAML(t, "", map[string]string{}); err == nil {
		t.Fatal("OPENAI_API_KEY 未設定でエラーになりません")
	}
}

func TestReadConfigFileMissingFileUsesDefaults(t *testing.T) {
	file, err := readConfigFile(filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(file, configFile{}) {
		t.Fatalf("file = %+v, want zero value", file)
	}
}

func TestReadConfigFileRejectsUnknownKeys(t *testing.T) {
	if _, err := loadFromYAML(t, "tts:\n  provder: voicevox\n", minimalEnv); err == nil {
		t.Fatal("未知のキーでエラーになりません")
	}
}

func TestReadConfigFileRejectsWrongTypes(t *testing.T) {
	if _, err := loadFromYAML(t, "tts:\n  voicevox:\n    speaker_id: abc\n", minimalEnv); err == nil {
		t.Fatal("型の誤りでエラーになりません")
	}
}

func TestBuildConfigDefaults(t *testing.T) {
	cfg := mustLoadFromYAML(t, "", minimalEnv)

	checks := map[string][2]string{
		"ResponsesModel":             {cfg.ResponsesModel, "gpt-5.4-nano-2026-03-17"},
		"RealtimeModel":              {cfg.RealtimeModel, "gpt-realtime-2.1"},
		"RealtimeVAD":                {cfg.RealtimeVAD, "semantic_vad"},
		"RealtimeNoiseReduction":     {cfg.RealtimeNoiseReduction, "far_field"},
		"RealtimeTranscriptionModel": {cfg.RealtimeTranscriptionModel, "gpt-4o-mini-transcribe"},
		"RealtimeEndpoint":           {cfg.RealtimeEndpoint, ""},
		"Memory.Model":               {cfg.Memory.Model, "gpt-5.6-luna"},
		"Memory.StorePath":           {cfg.Memory.StorePath, "data/memories.json"},
		"WakeModeStorePath":          {cfg.WakeModeStorePath, "data/wake-mode.json"},
		"AutomationStorePath":        {cfg.AutomationStorePath, "data/automations.json"},
		"LocalSTTEndpoint":           {cfg.LocalSTTEndpoint, "http://local-stt:8000/transcribe"},
		"TTSProvider":                {cfg.TTSProvider, "voicevox"},
		"Voicevox.Endpoint":          {cfg.Voicevox.Endpoint, "http://voicevox:50021"},
		"ElevenLabs.Model":           {cfg.ElevenLabs.Model, "eleven_v3"},
		"WSAddr":                     {cfg.WSAddr, ":8081"},
		"WebDistDir":                 {cfg.WebDistDir, "web/dist"},
	}
	for name, c := range checks {
		if c[0] != c[1] {
			t.Errorf("%s = %q, want %q", name, c[0], c[1])
		}
	}
	if cfg.Voicevox.SpeakerID != 1 || cfg.Voicevox.SpeedScale != nil {
		t.Errorf("Voicevox = %+v", cfg.Voicevox)
	}
	if cfg.ConversationIdleTimeout != 30*time.Minute {
		t.Errorf("ConversationIdleTimeout = %s", cfg.ConversationIdleTimeout)
	}
	if cfg.Memory.MemorizeAfter != 24*time.Hour || cfg.Memory.HistoryMaxBytes != 40000 {
		t.Errorf("Memory = %+v", cfg.Memory)
	}
	if len(cfg.RTCIceAdvertiseIPs) != 0 {
		t.Errorf("RTCIceAdvertiseIPs = %#v", cfg.RTCIceAdvertiseIPs)
	}
	if cfg.WakeEnergyThreshold != 0.005 {
		t.Errorf("WakeEnergyThreshold = %v, want 0.005", cfg.WakeEnergyThreshold)
	}
	w := cfg.WakeSettings
	if len(w.Words) != 3 || w.MaxPrefix != 4 || w.MinConfidence != 0.55 || w.MinSimilarity != 0.65 || w.ConversationTimeout != 5*time.Minute {
		t.Errorf("WakeSettings = %+v", w)
	}
}

func TestBuildConfigReadsYAMLValues(t *testing.T) {
	content := `
server:
  ws_addr: ":9090"
  web_dist_dir: dist
rtc:
  ice_advertise_ips: [" 192.168.0.100", "100.64.0.1", ""]
openai:
  responses_model: gpt-response
  realtime:
    model: comparison-model
    vad: server_vad
    transcription_model: gpt-transcribe
    endpoint: ws://127.0.0.1:1234
conversation:
  idle_timeout_seconds: 0
memory:
  model: gpt-memory
  store_path: /app/data/memories.json
  memorize_after_hours: 48
  history_max_bytes: 60000
automations:
  store_path: /tmp/automations.json
tts:
  provider: elevenlabs
  voicevox:
    endpoint: http://localhost:50021
    speaker_id: 0
    speed_scale: 1.25
  elevenlabs:
    voice_id: voice
    model: eleven_flash_v2_5
wake:
  store_path: /tmp/wake.json
  local_stt_endpoint: http://localhost:8000/transcribe
  words: [おきて, " 起きて ", "", オッケー]
  max_prefix: 0
  min_confidence: 0.4
  min_similarity: 0.7
  energy_threshold: 0.003
  conversation_timeout_seconds: 120
`
	env := map[string]string{
		"OPENAI_API_KEY":     "openai",
		"ELEVENLABS_API_KEY": "eleven",
	}
	cfg := mustLoadFromYAML(t, content, env)

	checks := map[string][2]string{
		"WSAddr":                     {cfg.WSAddr, ":9090"},
		"WebDistDir":                 {cfg.WebDistDir, "dist"},
		"ResponsesModel":             {cfg.ResponsesModel, "gpt-response"},
		"RealtimeModel":              {cfg.RealtimeModel, "comparison-model"},
		"RealtimeVAD":                {cfg.RealtimeVAD, "server_vad"},
		"RealtimeTranscriptionModel": {cfg.RealtimeTranscriptionModel, "gpt-transcribe"},
		"RealtimeEndpoint":           {cfg.RealtimeEndpoint, "ws://127.0.0.1:1234"},
		"Memory.Model":               {cfg.Memory.Model, "gpt-memory"},
		"Memory.StorePath":           {cfg.Memory.StorePath, "/app/data/memories.json"},
		"AutomationStorePath":        {cfg.AutomationStorePath, "/tmp/automations.json"},
		"TTSProvider":                {cfg.TTSProvider, "elevenlabs"},
		"Voicevox.Endpoint":          {cfg.Voicevox.Endpoint, "http://localhost:50021"},
		"ElevenLabs.APIKey":          {cfg.ElevenLabs.APIKey, "eleven"},
		"ElevenLabs.VoiceID":         {cfg.ElevenLabs.VoiceID, "voice"},
		"ElevenLabs.Model":           {cfg.ElevenLabs.Model, "eleven_flash_v2_5"},
		"WakeModeStorePath":          {cfg.WakeModeStorePath, "/tmp/wake.json"},
		"LocalSTTEndpoint":           {cfg.LocalSTTEndpoint, "http://localhost:8000/transcribe"},
	}
	for name, c := range checks {
		if c[0] != c[1] {
			t.Errorf("%s = %q, want %q", name, c[0], c[1])
		}
	}
	if cfg.Voicevox.SpeakerID != 0 {
		t.Errorf("Voicevox.SpeakerID = %d, want 0", cfg.Voicevox.SpeakerID)
	}
	if cfg.Voicevox.SpeedScale == nil || *cfg.Voicevox.SpeedScale != 1.25 {
		t.Errorf("Voicevox.SpeedScale = %v, want 1.25", cfg.Voicevox.SpeedScale)
	}
	if cfg.ConversationIdleTimeout != 0 {
		t.Errorf("ConversationIdleTimeout = %s, want 0", cfg.ConversationIdleTimeout)
	}
	if cfg.Memory.MemorizeAfter != 48*time.Hour || cfg.Memory.HistoryMaxBytes != 60000 {
		t.Errorf("Memory = %+v", cfg.Memory)
	}
	if want := []string{"192.168.0.100", "100.64.0.1"}; !reflect.DeepEqual(cfg.RTCIceAdvertiseIPs, want) {
		t.Errorf("RTCIceAdvertiseIPs = %#v, want %#v", cfg.RTCIceAdvertiseIPs, want)
	}
	if cfg.WakeEnergyThreshold != 0.003 {
		t.Errorf("WakeEnergyThreshold = %v, want 0.003", cfg.WakeEnergyThreshold)
	}
	w := cfg.WakeSettings
	if want := []string{"おきて", "起きて", "オッケー"}; !reflect.DeepEqual(w.Words, want) || w.MaxPrefix != 0 || w.MinConfidence != 0.4 || w.MinSimilarity != 0.7 || w.ConversationTimeout != 2*time.Minute {
		t.Errorf("WakeSettings = %+v", w)
	}
}

func TestBuildConfigDerivesStorePathsFromMemoryStorePath(t *testing.T) {
	cfg := mustLoadFromYAML(t, "memory:\n  store_path: /app/data/memories.json\n", minimalEnv)
	if cfg.WakeModeStorePath != "/app/data/wake-mode.json" || cfg.AutomationStorePath != "/app/data/automations.json" {
		t.Fatalf("store paths = %q, %q", cfg.WakeModeStorePath, cfg.AutomationStorePath)
	}
}

func TestBuildConfigRTCIceAdvertiseIPsEnvOverridesYAML(t *testing.T) {
	env := map[string]string{"OPENAI_API_KEY": "openai", "RTC_ICE_ADVERTISE_IPS": " 192.168.0.100,100.64.0.1, ,192.168.0.101 "}
	cfg := mustLoadFromYAML(t, "rtc:\n  ice_advertise_ips: [10.0.0.1]\n", env)
	want := []string{"192.168.0.100", "100.64.0.1", "192.168.0.101"}
	if !reflect.DeepEqual(cfg.RTCIceAdvertiseIPs, want) {
		t.Fatalf("RTCIceAdvertiseIPs = %#v, want %#v", cfg.RTCIceAdvertiseIPs, want)
	}
}

func TestBuildConfigFallsBackOnOutOfRangeNumbers(t *testing.T) {
	cfg := mustLoadFromYAML(t, "conversation:\n  idle_timeout_seconds: -1\ntts:\n  voicevox:\n    speed_scale: -1\n", minimalEnv)
	if cfg.ConversationIdleTimeout != 30*time.Minute {
		t.Errorf("ConversationIdleTimeout = %s, want default", cfg.ConversationIdleTimeout)
	}
	if cfg.Voicevox.SpeedScale != nil {
		t.Errorf("Voicevox.SpeedScale = %v, want nil", *cfg.Voicevox.SpeedScale)
	}
}

// config.sample.yamlがそのままでも、コメントアウトされた項目をすべて有効にしても読み込めることを確かめる。
// 構造体に無い項目がサンプルに書かれていると、未知のキーとしてエラーになる。
func TestConfigSampleYAMLIsLoadable(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "config.sample.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loadFromYAML(t, string(data), minimalEnv); err != nil {
		t.Fatalf("config.sample.yaml: %v", err)
	}

	keyLine := regexp.MustCompile(`^( *)# ([a-z_]+:.*)$`)
	nestedLine := regexp.MustCompile(`^( *)#   (- .*|[a-z0-9_]+: .*)$`)
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if m := keyLine.FindStringSubmatch(line); m != nil {
			lines[i] = m[1] + m[2]
		} else if m := nestedLine.FindStringSubmatch(line); m != nil {
			lines[i] = m[1] + "  " + m[2]
		}
	}
	cfg, err := loadFromYAML(t, strings.Join(lines, "\n"), minimalEnv)
	if err != nil {
		t.Fatalf("config.sample.yaml（全項目有効）: %v", err)
	}
	if cfg.Voicevox.SpeedScale == nil || len(cfg.RTCIceAdvertiseIPs) != 1 {
		t.Fatalf("コメントアウトされた項目が有効になっていません: %+v", cfg)
	}
}

func TestBuildConfigUsesDefaultSystemPrompt(t *testing.T) {
	cfg := mustLoadFromYAML(t, "", minimalEnv)
	if cfg.SystemPrompt != defaultSystemPrompt || !strings.Contains(cfg.SystemPrompt, "会話型AI") {
		t.Fatalf("SystemPrompt が既定のプロンプトになっていません: %q", cfg.SystemPrompt)
	}
}

func TestBuildConfigAppendsAdditionalSystemPrompt(t *testing.T) {
	cfg := mustLoadFromYAML(t, "additional_system_prompt: |\n  ユーザーの名前は太郎です。\n", minimalEnv)
	want := strings.TrimRight(defaultSystemPrompt, "\n") + "\n\nユーザーの名前は太郎です。\n"
	if cfg.SystemPrompt != want {
		t.Fatalf("SystemPrompt の末尾 = %q", cfg.SystemPrompt[len(cfg.SystemPrompt)-40:])
	}
}

func TestLegacyEnvWarnings(t *testing.T) {
	env := map[string]string{"TTS_PROVIDER": "voicevox", "GOOGLE_REDIRECT_URL": "http://localhost/callback", "WS_ADDR": " ", "OPENAI_API_KEY": "secret"}
	got := legacyEnvWarnings(func(name string) string { return env[name] })
	if len(got) != 2 || !strings.Contains(got[0], "GOOGLE_REDIRECT_URL") || !strings.Contains(got[0], "plugins.googlecalendar.redirect_url") || !strings.Contains(got[1], "TTS_PROVIDER") {
		t.Fatalf("warnings = %#v", got)
	}
}

// 発話とみなす音量の範囲外の値は、起動時にエラーにする。
func TestBuildConfigRejectsOutOfRangeWakeEnergyThreshold(t *testing.T) {
	for _, value := range []string{"0", "-0.001", "1.5"} {
		if _, err := loadFromYAML(t, "wake:\n  energy_threshold: "+value+"\n", minimalEnv); err == nil {
			t.Errorf("範囲外の音量しきい値を受理しました: %s", value)
		}
	}
	cfg, err := loadFromYAML(t, "wake:\n  energy_threshold: 1\n", minimalEnv)
	if err != nil || cfg.WakeEnergyThreshold != 1 {
		t.Errorf("WakeEnergyThreshold = %v, err = %v", cfg.WakeEnergyThreshold, err)
	}
}

func TestPluginConfigs(t *testing.T) {
	cfg := mustLoadFromYAML(t, "plugins:\n  sample:\n    url: http://example.test\n    count: 2\n  other: {}\n", minimalEnv)
	if got := cfg.PluginConfigNames(); !reflect.DeepEqual(got, []string{"other", "sample"}) {
		t.Fatalf("PluginConfigNames = %v", got)
	}
	var sample struct {
		URL   string `yaml:"url"`
		Count int    `yaml:"count"`
	}
	if err := cfg.DecodePluginConfig("sample", &sample); err != nil {
		t.Fatal(err)
	}
	if sample.URL != "http://example.test" || sample.Count != 2 {
		t.Fatalf("sample = %+v", sample)
	}

	// 書かれていないプラグインは、値を変えずにエラーにしない。
	missing := struct {
		URL string `yaml:"url"`
	}{URL: "既定値"}
	if err := cfg.DecodePluginConfig("missing", &missing); err != nil || missing.URL != "既定値" {
		t.Fatalf("missing = %+v, err = %v", missing, err)
	}

	// プラグインの設定の中の未知のキーや型の誤りは、エラーにする。
	var wrong struct {
		URL string `yaml:"url"`
	}
	if err := cfg.DecodePluginConfig("sample", &wrong); err == nil {
		t.Fatal("未知のキーでエラーになりません")
	}
	var wrongType struct {
		URL   string `yaml:"url"`
		Count bool   `yaml:"count"`
	}
	if err := cfg.DecodePluginConfig("sample", &wrongType); err == nil {
		t.Fatal("型の誤りでエラーになりません")
	}
}

func TestLoadVoicevoxSpeakerName(t *testing.T) {
	cases := []struct {
		yaml string
		want string
	}{
		{"", "ずんだもん"},
		{"tts:\n  voicevox:\n    speaker_id: 2\n", ""},
		{"tts:\n  voicevox:\n    speaker_id: 2\n    speaker_name: \" 四国めたん \"\n", "四国めたん"},
		{"tts:\n  voicevox:\n    speaker_name: 春日部つむぎ\n", "春日部つむぎ"},
	}
	for _, c := range cases {
		cfg := mustLoadFromYAML(t, c.yaml, minimalEnv)
		if cfg.Voicevox.SpeakerName != c.want {
			t.Errorf("yaml=%q: SpeakerName = %q, want %q", c.yaml, cfg.Voicevox.SpeakerName, c.want)
		}
	}
}
