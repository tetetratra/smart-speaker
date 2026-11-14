package tts

import (
	"context"
	"encoding/base64"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	types "github.com/tetetratra/smart-speaker/internal/types"
)

type fakeSynthesizer struct {
	mu    sync.Mutex
	pcm   []byte
	err   error
	calls []string
}

func (f *fakeSynthesizer) Name() string {
	return "fake"
}

func (f *fakeSynthesizer) SynthesizeSpeech(_ context.Context, text string) (synthesizedSpeech, error) {
	f.mu.Lock()
	f.calls = append(f.calls, text)
	f.mu.Unlock()
	if f.err != nil {
		return synthesizedSpeech{}, f.err
	}
	return synthesizedSpeech{PCM: f.pcm}, nil
}

func TestStagePassesThroughNonSpeechTimelineItemsAndAgentTimelineEnd(t *testing.T) {
	synth := &fakeSynthesizer{}
	stage, err := newStageWithSynthesizer(synth)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go stage.Run(ctx)
	defer stage.Close()

	// speech 以外の item は音声化せず、そのまま下流へ流す。
	otherItem := types.TimelineItem{Kind: "other", Text: "読み上げない"}
	go func() {
		stage.Upstream <- types.EventTimelineItem{TimelineItem: otherItem}
		stage.Upstream <- types.EventAgentTimelineEnd{}
	}()

	assertEvent(t, stage.Downstream, types.EventTimelineItem{TimelineItem: otherItem})
	assertEvent(t, stage.Downstream, types.EventAgentTimelineEnd{})
	if len(synth.Calls()) != 0 {
		t.Fatalf("expected no synthesize calls, got %v", synth.Calls())
	}
}

func TestStageEmitsPlayableSpeech(t *testing.T) {
	pcm := []byte{1, 2, 3, 4}
	synth := &fakeSynthesizer{pcm: pcm}
	stage, err := newStageWithSynthesizer(synth)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go stage.Run(ctx)
	defer stage.Close()

	item := types.TimelineItem{
		Kind:         types.TimelineKindSpeech,
		Text:         "こんにちは",
		GenerationID: 1,
		SequenceID:   "2",
	}
	stage.Upstream <- types.EventTimelineItem{TimelineItem: item}

	evt := receiveEvent(t, stage.Downstream)
	if !types.Is[types.EventPlayableSpeech](evt) {
		t.Fatalf("event = %s, want EventPlayableSpeech", types.EventName(evt))
	}
	playableEvent, ok := evt.(types.EventPlayableSpeech)
	playable := playableEvent.PlayableSpeech
	if !ok {
		t.Fatalf("payload type = %T, want types.PlayableSpeech", evt)
	}
	if playable.Audio != base64.StdEncoding.EncodeToString(pcm) {
		t.Fatalf("audio = %q, want encoded pcm", playable.Audio)
	}
	if playable.DurationSeconds != ttsDurationSeconds(int64(len(pcm))) {
		t.Fatalf("duration = %v, want %v", playable.DurationSeconds, ttsDurationSeconds(int64(len(pcm))))
	}
	if !reflect.DeepEqual(playable.OriginalTimeline, item) {
		t.Fatalf("original timeline = %#v, want %#v", playable.OriginalTimeline, item)
	}
	if len(synth.Calls()) != 1 || synth.Calls()[0] != item.Text {
		t.Fatalf("synthesize calls = %v, want [%q]", synth.Calls(), item.Text)
	}
}

func TestStageDropsBlankSpeech(t *testing.T) {
	synth := &fakeSynthesizer{pcm: []byte{1, 2}}
	stage, err := newStageWithSynthesizer(synth)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go stage.Run(ctx)
	defer stage.Close()

	stage.Upstream <- types.EventTimelineItem{TimelineItem: types.TimelineItem{Kind: types.TimelineKindSpeech, Text: " \n\t "}}

	assertNoEvent(t, stage.Downstream)
	if len(synth.Calls()) != 0 {
		t.Fatalf("expected no synthesize calls, got %v", synth.Calls())
	}
}

func TestStageDropsSpeechWhenSynthesisFails(t *testing.T) {
	synth := &fakeSynthesizer{err: errors.New("failed")}
	stage, err := newStageWithSynthesizer(synth)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go stage.Run(ctx)
	defer stage.Close()

	stage.Upstream <- types.EventTimelineItem{TimelineItem: types.TimelineItem{Kind: types.TimelineKindSpeech, Text: "こんにちは"}}

	assertNoEvent(t, stage.Downstream)
	if len(synth.Calls()) != 1 {
		t.Fatalf("synthesize calls = %v, want one call", synth.Calls())
	}
}

func assertEvent(t *testing.T, events <-chan types.Event, want types.Event) {
	t.Helper()
	got := receiveEvent(t, events)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("event = %#v, want %#v", got, want)
	}
}

func receiveEvent(t *testing.T, events <-chan types.Event) types.Event {
	t.Helper()
	select {
	case evt := <-events:
		return evt
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
	return nil
}

func assertNoEvent(t *testing.T, events <-chan types.Event) {
	t.Helper()
	select {
	case evt := <-events:
		t.Fatalf("unexpected event: %#v", evt)
	case <-time.After(20 * time.Millisecond):
	}
}

func (f *fakeSynthesizer) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}
