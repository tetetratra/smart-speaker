package tts

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	types "github.com/tetetratra/smart-speaker/internal/types"
)

type blockingSynthesizer struct {
	started chan struct{}
	release chan struct{}
}

func (s *blockingSynthesizer) Name() string { return ProviderVoicevox }
func (s *blockingSynthesizer) SynthesizeSpeech(ctx context.Context, _ string) (synthesizedSpeech, error) {
	close(s.started)
	select {
	case <-s.release:
		return synthesizedSpeech{PCM: []byte{1, 2}}, nil
	case <-ctx.Done():
		return synthesizedSpeech{}, ctx.Err()
	}
}

func TestSelectorSwitchDuringSpeechPreservesCurrentAndChangesNext(t *testing.T) {
	first := &blockingSynthesizer{started: make(chan struct{}), release: make(chan struct{})}
	next := &fakeSynthesizer{pcm: []byte{3, 4}}
	selector := &Selector{selected: ProviderVoicevox, synthesizers: map[string]speechSynthesizer{ProviderVoicevox: first, ProviderElevenLabs: next}}
	stage, err := NewStage(Config{Selector: selector})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go stage.Run(ctx)
	defer stage.Close()
	stage.Upstream <- types.Event{Kind: types.EventTimelineItem, Payload: types.TimelineItem{Kind: types.TimelineKindSpeech, Text: "切り替え前"}}
	select {
	case <-first.started:
	case <-ctx.Done():
		t.Fatal("最初の音声生成が開始されませんでした")
	}
	switched := make(chan error, 1)
	go func() { _, err := selector.Select(ProviderElevenLabs); switched <- err }()
	select {
	case err := <-switched:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("生成中の音声が切り替えを妨げました")
	}
	close(first.release)
	if got := receiveEvent(t, stage.Downstream).Payload.(types.PlayableSpeech).Audio; got != base64.StdEncoding.EncodeToString([]byte{1, 2}) {
		t.Fatal("生成中の音声が変更されました")
	}
	stage.Upstream <- types.Event{Kind: types.EventTimelineItem, Payload: types.TimelineItem{Kind: types.TimelineKindSpeech, Text: "切り替え後"}}
	if got := receiveEvent(t, stage.Downstream).Payload.(types.PlayableSpeech).Audio; got != base64.StdEncoding.EncodeToString(next.pcm) {
		t.Fatal("次の音声に選択が反映されていません")
	}
}

func TestSelectorRejectsUnconfiguredProviderWithoutChangingSelection(t *testing.T) {
	s, err := NewSelector(Config{Voicevox: VoicevoxConfig{Endpoint: "http://localhost:50021"}})
	if err != nil {
		t.Fatal(err)
	}
	if s.Snapshot().Providers[1].Available {
		t.Fatal("未設定のElevenLabsが選択可能です")
	}
	for _, provider := range []string{ProviderElevenLabs, "unknown", ""} {
		if _, err := s.Select(provider); err == nil {
			t.Fatalf("%qが受理されました", provider)
		}
		if s.Snapshot().Provider != ProviderVoicevox {
			t.Fatal("エラー時に選択が変わりました")
		}
	}
}
