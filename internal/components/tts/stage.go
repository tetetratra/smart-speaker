package tts

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/tetetratra/smart-speaker/internal/graph"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

type streamTTS struct {
	synthesizer speechSynthesizer
	upstream    chan types.Event
	downstream  chan types.Event
	once        sync.Once
}

func newStageWithSynthesizer(synthesizer speechSynthesizer) (*graph.Stage, error) {
	if synthesizer == nil {
		return nil, fmt.Errorf("tts: synthesizer is required")
	}
	t := &streamTTS{
		synthesizer: synthesizer,
		upstream:    make(chan types.Event, graph.DefaultChannelBufferSize),
		downstream:  make(chan types.Event, graph.DefaultChannelBufferSize),
	}
	return &graph.Stage{
		Upstream:   t.upstream,
		Downstream: t.downstream,
		Run:        t.run,
		CloseFn:    t.close,
	}, nil
}

func (t *streamTTS) run(ctx context.Context) {
	defer close(t.downstream)
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-t.upstream:
			if !ok {
				return
			}
			if evt.Kind == types.EventAgentTimelineEnd {
				t.emit(ctx, evt)
				continue
			}
			if evt.Kind != types.EventTimelineItem {
				continue
			}
			item, ok := evt.Payload.(types.TimelineItem)
			if !ok {
				continue
			}
			if item.Kind != types.TimelineKindSpeech {
				t.emit(ctx, evt)
				continue
			}
			if strings.TrimSpace(item.Text) == "" {
				continue
			}
			t.handleSpeech(ctx, item)
		}
	}
}

func (t *streamTTS) emit(ctx context.Context, evt types.Event) {
	select {
	case <-ctx.Done():
	case t.downstream <- evt:
	}
}

func (t *streamTTS) handleSpeech(ctx context.Context, item types.TimelineItem) {
	speech, err := t.synthesizer.SynthesizeSpeech(ctx, item.Text)
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("%s: synthesize error: %v", t.synthesizer.Name(), err)
		}
		return
	}
	duration := ttsDurationSeconds(int64(len(speech.PCM)))
	log.Printf("%s: tts duration=%.3fs bytes=%d", t.synthesizer.Name(), duration, len(speech.PCM))
	playable := types.PlayableSpeech{
		GenerationID:     item.GenerationID,
		SequenceID:       item.SequenceID,
		Text:             item.Text,
		Audio:            base64.StdEncoding.EncodeToString(speech.PCM),
		DurationSeconds:  duration,
		OriginalTimeline: item,
	}
	select {
	case <-ctx.Done():
	case t.downstream <- types.Event{Kind: types.EventPlayableSpeech, Payload: playable}:
	}
}

func (t *streamTTS) close() error {
	t.once.Do(func() {
		close(t.upstream)
	})
	return nil
}
