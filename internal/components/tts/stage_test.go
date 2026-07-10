package tts

import (
	"context"
	"encoding/base64"
	"errors"
	"reflect"
	"testing"
	"time"

	types "github.com/tetetratra/smart-speaker/internal/types"
)

type fakeSynthesizer struct {
	pcm   []byte
	err   error
	calls []string
}

func (f *fakeSynthesizer) Name() string {
	return "fake"
}

func (f *fakeSynthesizer) SynthesizeSpeech(_ context.Context, text string) (synthesizedSpeech, error) {
	f.calls = append(f.calls, text)
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

	waitItem := types.TimelineItem{Kind: types.TimelineKindWait, Sec: 1}
	toolItem := types.TimelineItem{Kind: types.TimelineKindTool, ToolName: "timer"}
	stage.Upstream <- types.Event{Kind: types.EventTimelineItem, Payload: waitItem}
	stage.Upstream <- types.Event{Kind: types.EventTimelineItem, Payload: toolItem}
	stage.Upstream <- types.Event{Kind: types.EventAgentTimelineEnd}

	assertEvent(t, stage.Downstream, types.Event{Kind: types.EventTimelineItem, Payload: waitItem})
	assertEvent(t, stage.Downstream, types.Event{Kind: types.EventTimelineItem, Payload: toolItem})
	assertEvent(t, stage.Downstream, types.Event{Kind: types.EventAgentTimelineEnd})
	if len(synth.calls) != 0 {
		t.Fatalf("expected no synthesize calls, got %v", synth.calls)
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
	stage.Upstream <- types.Event{Kind: types.EventTimelineItem, Payload: item}

	evt := receiveEvent(t, stage.Downstream)
	if evt.Kind != types.EventPlayableSpeech {
		t.Fatalf("event kind = %s, want %s", evt.Kind, types.EventPlayableSpeech)
	}
	playable, ok := evt.Payload.(types.PlayableSpeech)
	if !ok {
		t.Fatalf("payload type = %T, want types.PlayableSpeech", evt.Payload)
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
	if len(synth.calls) != 1 || synth.calls[0] != item.Text {
		t.Fatalf("synthesize calls = %v, want [%q]", synth.calls, item.Text)
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

	stage.Upstream <- types.Event{
		Kind:    types.EventTimelineItem,
		Payload: types.TimelineItem{Kind: types.TimelineKindSpeech, Text: " \n\t "},
	}

	assertNoEvent(t, stage.Downstream)
	if len(synth.calls) != 0 {
		t.Fatalf("expected no synthesize calls, got %v", synth.calls)
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

	stage.Upstream <- types.Event{
		Kind:    types.EventTimelineItem,
		Payload: types.TimelineItem{Kind: types.TimelineKindSpeech, Text: "こんにちは"},
	}

	assertNoEvent(t, stage.Downstream)
	if len(synth.calls) != 1 {
		t.Fatalf("synthesize calls = %v, want one call", synth.calls)
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
	return types.Event{}
}

func assertNoEvent(t *testing.T, events <-chan types.Event) {
	t.Helper()
	select {
	case evt := <-events:
		t.Fatalf("unexpected event: %#v", evt)
	case <-time.After(20 * time.Millisecond):
	}
}
