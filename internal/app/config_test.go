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
		"GoogleCalendar.RedirectURL": {cfg.GoogleCalendar.RedirectURL, "http://localhost:8081/oauth/google/callback"},
		"GoogleCalendar.Scope":       {cfg.GoogleCalendar.Scopes[0], "https://www.googleapis.com/auth/calendar.events"},
		"GoogleCalendar.TokenPath":   {cfg.GoogleCalendar.TokenPath, "data/google_oauth_token.json"},
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
switchbot:
  device_map:
    hub2: hub-device
google_calendar:
  redirect_url: http://example.test/callback
  scope: https://www.googleapis.com/auth/calendar.readonly
  token_path: /tmp/token.json
`
	env := map[string]string{
		"OPENAI_API_KEY":       "openai",
		"ELEVENLABS_API_KEY":   "eleven",
		"SWITCHBOT_TOKEN":      "token",
		"SWITCHBOT_SECRET":     "secret",
		"GOOGLE_CLIENT_ID":     "client-id",
		"GOOGLE_CLIENT_SECRET": "client-secret",
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
		"SwitchBot.Token":            {cfg.SwitchBot.Token, "token"},
		"SwitchBot.Secret":           {cfg.SwitchBot.Secret, "secret"},
		"GoogleCalendar.ClientID":    {cfg.GoogleCalendar.ClientID, "client-id"},
		"GoogleCalendar.Secret":      {cfg.GoogleCalendar.ClientSecret, "client-secret"},
		"GoogleCalendar.RedirectURL": {cfg.GoogleCalendar.RedirectURL, "http://example.test/callback"},
		"GoogleCalendar.Scope":       {cfg.GoogleCalendar.Scopes[0], "https://www.googleapis.com/auth/calendar.readonly"},
		"GoogleCalendar.TokenPath":   {cfg.GoogleCalendar.TokenPath, "/tmp/token.json"},
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
	if want := []string{"192.168.0.100", "100.64.0.1"}; !reflect.DeepEqual(cfg.RTCIceAdvertiseIPs, want) {
		t.Errorf("RTCIceAdvertiseIPs = %#v, want %#v", cfg.RTCIceAdvertiseIPs, want)
	}
	if want := map[string]string{"hub2": "hub-device"}; !reflect.DeepEqual(cfg.SwitchBot.DeviceMap, want) {
		t.Errorf("SwitchBot.DeviceMap = %#v", cfg.SwitchBot.DeviceMap)
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
	if cfg.WakeModeStorePath != "/app/data/wake-mode.json" || cfg.AutomationStorePath != "/app/data/automations.json" || cfg.GoogleCalendar.TokenPath != "/app/data/google_oauth_token.json" {
		t.Fatalf("store paths = %q, %q, %q", cfg.WakeModeStorePath, cfg.AutomationStorePath, cfg.GoogleCalendar.TokenPath)
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

// 呼びかけなしで話せる時間は、無操作リセットより短くなければ起動時にエラーにする。
// 無操作リセットが無効（0）なら比べない。
func TestBuildConfigRequiresWakeTimeoutShorterThanIdleTimeout(t *testing.T) {
	for _, content := range []string{
		"conversation:\n  idle_timeout_seconds: 300\nwake:\n  conversation_timeout_seconds: 300\n",
		"conversation:\n  idle_timeout_seconds: 300\nwake:\n  conversation_timeout_seconds: 301\n",
		"conversation:\n  idle_timeout_seconds: 240\n",
	} {
		if _, err := loadFromYAML(t, content, minimalEnv); err == nil {
			t.Errorf("無操作リセット以上の会話中の時間を受理しました: %q", content)
		}
	}
	for _, content := range []string{
		"conversation:\n  idle_timeout_seconds: 300\nwake:\n  conversation_timeout_seconds: 299\n",
		"conversation:\n  idle_timeout_seconds: 0\nwake:\n  conversation_timeout_seconds: 3600\n",
	} {
		if _, err := loadFromYAML(t, content, minimalEnv); err != nil {
			t.Errorf("%q: %v", content, err)
		}
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
	if cfg.Voicevox.SpeedScale == nil || len(cfg.RTCIceAdvertiseIPs) != 1 || len(cfg.SwitchBot.DeviceMap) != 1 {
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
	env := map[string]string{"TTS_PROVIDER": "voicevox", "SWITCHBOT_DEVICE_MAP": "{}", "WS_ADDR": " ", "OPENAI_API_KEY": "secret"}
	got := legacyEnvWarnings(func(name string) string { return env[name] })
	if len(got) != 2 || !strings.Contains(got[0], "SWITCHBOT_DEVICE_MAP") || !strings.Contains(got[0], "switchbot.device_map") || !strings.Contains(got[1], "TTS_PROVIDER") {
		t.Fatalf("warnings = %#v", got)
	}
}

func TestBuildConfigReadsAgentBridgeURLFromYAMLAndTokenFromEnv(t *testing.T) {
	cfg := mustLoadFromYAML(t, "", minimalEnv)
	if cfg.AgentBridge != (AgentBridgeConfig{}) {
		t.Fatalf("AgentBridge = %#v, want empty", cfg.AgentBridge)
	}
	env := map[string]string{"OPENAI_API_KEY": "openai", "AGENT_BRIDGE_TOKEN": " secret "}
	cfg = mustLoadFromYAML(t, "agent_bridge:\n  url: http://100.64.0.2:8787\n", env)
	want := AgentBridgeConfig{URL: "http://100.64.0.2:8787", Token: "secret"}
	if cfg.AgentBridge != want {
		t.Fatalf("AgentBridge = %#v, want %#v", cfg.AgentBridge, want)
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
