package tts

import (
	"fmt"
	"strings"
	"sync"
)

// Selector は音声生成ごとに参照するTTSの選択状態を共有します。
type Selector struct {
	mu           sync.RWMutex
	selected     string
	synthesizers map[string]speechSynthesizer
	// voicevoxCreditは、VOICEVOX の利用規約が求めるクレジット表記。
	voicevoxCredit string
}

type ProviderOption struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Available bool   `json:"available"`
	// Creditは、そのTTSを使うときに画面へ表示するクレジット表記。不要なTTSでは空。
	Credit string `json:"credit,omitempty"`
}

type Selection struct {
	Provider  string           `json:"provider"`
	Providers []ProviderOption `json:"providers"`
}

func NewSelector(cfg Config) (*Selector, error) {
	initial, err := newSynthesizer(cfg)
	if err != nil {
		return nil, err
	}
	s := &Selector{selected: initial.Name(), synthesizers: map[string]speechSynthesizer{initial.Name(): initial}, voicevoxCredit: voicevoxCredit(cfg.Voicevox.SpeakerName)}
	for _, provider := range []string{ProviderVoicevox, ProviderElevenLabs} {
		if _, ok := s.synthesizers[provider]; ok {
			continue
		}
		candidate := cfg
		candidate.Provider = provider
		if synth, err := newSynthesizer(candidate); err == nil {
			s.synthesizers[provider] = synth
		}
	}
	return s, nil
}

func (s *Selector) Snapshot() Selection {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Selection{Provider: s.selected, Providers: []ProviderOption{
		{ID: ProviderVoicevox, Label: "VOICEVOX", Available: s.synthesizers[ProviderVoicevox] != nil, Credit: s.voicevoxCredit},
		{ID: ProviderElevenLabs, Label: "ElevenLabs", Available: s.synthesizers[ProviderElevenLabs] != nil},
	}}
}

func (s *Selector) Select(provider string) (Selection, error) {
	provider = strings.TrimSpace(provider)
	s.mu.Lock()
	if s.synthesizers[provider] == nil {
		s.mu.Unlock()
		return Selection{}, fmt.Errorf("選択できないTTSです。サーバーの設定を確認してください")
	}
	s.selected = provider
	s.mu.Unlock()
	return s.Snapshot(), nil
}

func (s *Selector) current() speechSynthesizer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.synthesizers[s.selected]
}

// voicevoxCreditは、話者名が分かれば「VOICEVOX:話者名」、分からなければ「VOICEVOX」を返す。
// 話者名を設定と一致させるのは運用者の責任とし、ここでは検証しない。
func voicevoxCredit(speakerName string) string {
	speakerName = strings.TrimSpace(speakerName)
	if speakerName == "" {
		return "VOICEVOX"
	}
	return "VOICEVOX:" + speakerName
}
