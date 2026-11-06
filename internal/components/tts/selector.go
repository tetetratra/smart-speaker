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
}

type ProviderOption struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Available bool   `json:"available"`
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
	s := &Selector{selected: initial.Name(), synthesizers: map[string]speechSynthesizer{initial.Name(): initial}}
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
		{ID: ProviderVoicevox, Label: "VOICEVOX", Available: s.synthesizers[ProviderVoicevox] != nil},
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
