package tts

import "testing"

func TestNewSynthesizerDefaultsToElevenLabs(t *testing.T) {
	synth, err := newSynthesizer(Config{
		ElevenLabs: ElevenLabsConfig{APIKey: "key", Voice: "voice"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if synth.Name() != ProviderElevenLabs {
		t.Fatalf("provider = %s, want %s", synth.Name(), ProviderElevenLabs)
	}
}

func TestNewSynthesizerSupportsVoicevox(t *testing.T) {
	synth, err := newSynthesizer(Config{
		Provider: ProviderVoicevox,
		Voicevox: VoicevoxConfig{
			Endpoint:  "http://voicevox:50021",
			SpeakerID: 1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if synth.Name() != ProviderVoicevox {
		t.Fatalf("provider = %s, want %s", synth.Name(), ProviderVoicevox)
	}
}

func TestNewSynthesizerRejectsUnknownProvider(t *testing.T) {
	_, err := newSynthesizer(Config{Provider: "unknown"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNewSynthesizerRequiresElevenLabsConfigOnlyForElevenLabs(t *testing.T) {
	if _, err := newSynthesizer(Config{Provider: ProviderElevenLabs}); err == nil {
		t.Fatal("expected elevenlabs config error")
	}
	if _, err := newSynthesizer(Config{
		Provider: ProviderVoicevox,
		Voicevox: VoicevoxConfig{
			Endpoint:  "http://voicevox:50021",
			SpeakerID: 1,
		},
	}); err != nil {
		t.Fatalf("voicevox should not require elevenlabs config: %v", err)
	}
}

func TestNewSynthesizerRejectsInvalidVoicevoxConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  VoicevoxConfig
	}{
		{name: "missing endpoint", cfg: VoicevoxConfig{SpeakerID: 1}},
		{name: "negative speaker", cfg: VoicevoxConfig{Endpoint: "http://voicevox:50021", SpeakerID: -1}},
		{name: "invalid speed", cfg: VoicevoxConfig{Endpoint: "http://voicevox:50021", SpeakerID: 1, SpeedScale: ptrFloat(0)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newSynthesizer(Config{Provider: ProviderVoicevox, Voicevox: tt.cfg})
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func ptrFloat(v float64) *float64 {
	return &v
}
