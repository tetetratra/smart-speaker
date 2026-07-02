package rtcvad

import (
	"context"
	"testing"
	"time"

	"github.com/tetetratra/smart-speaker/internal/states/generation"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

func TestHandleAudioFrameStartsSpeechAndEmitsPrebuffer(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	store := generation.NewStore()
	s := &stage{
		generation: store,
		ctx:        context.Background(),
		downstream: make(chan types.Event, 16),
		peers: map[string]*peerState{
			"peer-1": {speechThreshold: adaptiveVADMinThreshold, speechThresholdUpdatedAt: now},
		},
	}
	firstPCM := []byte{1, 0, 2, 0}

	s.handleAudioFrame(types.RTCPeerAudioFrame{
		PeerID:     "peer-1",
		Samples:    []int16{300, 300},
		PCM:        firstPCM,
		SampleRate: webrtcSampleRate,
		DurationMs: 100,
		CapturedAt: now,
	})
	s.handleAudioFrame(types.RTCPeerAudioFrame{
		PeerID:     "peer-1",
		Samples:    []int16{300, 300},
		PCM:        []byte{3, 0, 4, 0},
		SampleRate: webrtcSampleRate,
		DurationMs: 100,
		CapturedAt: now.Add(100 * time.Millisecond),
	})

	events := drainEvents(s.downstream)
	if len(events) != 4 {
		t.Fatalf("expected status, speech-start, audio-start and frame events, got %d: %#v", len(events), events)
	}
	if events[0].Kind != types.EventRTCVADStatus {
		t.Fatalf("expected first event to be VAD status, got %s", events[0].Kind)
	}
	if events[1].Kind != types.EventSpeechStart {
		t.Fatalf("expected speech start event, got %s", events[1].Kind)
	}
	start, ok := events[2].Payload.(types.RTCSpeechAudio)
	if events[2].Kind != types.EventRTCSpeechAudio || !ok || start.Type != types.RTCSpeechAudioStart {
		t.Fatalf("expected speech audio start event, got %#v", events[2])
	}
	if string(start.Prebuffer) != string(firstPCM) {
		t.Fatalf("expected prebuffer %v, got %v", firstPCM, start.Prebuffer)
	}
	frame, ok := events[3].Payload.(types.RTCSpeechAudio)
	if events[3].Kind != types.EventRTCSpeechAudio || !ok || frame.Type != types.RTCSpeechAudioFrame {
		t.Fatalf("expected speech audio frame event, got %#v", events[3])
	}
	if s.activeSpeakerID != "peer-1" {
		t.Fatalf("expected active speaker peer-1, got %q", s.activeSpeakerID)
	}
	if got := store.Current(); got != 1 {
		t.Fatalf("generation = %d, want 1", got)
	}
}

func TestHandleAudioFrameRequiresSustainedSpeechBeforeStart(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	store := generation.NewStore()
	s := &stage{
		generation: store,
		ctx:        context.Background(),
		downstream: make(chan types.Event, 16),
		peers: map[string]*peerState{
			"peer-1": {speechThreshold: adaptiveVADMinThreshold, speechThresholdUpdatedAt: now},
		},
	}

	s.handleAudioFrame(audioFrame("peer-1", 300, vadStartThreshold-1, now))
	events := drainEvents(s.downstream)
	if len(events) != 1 || events[0].Kind != types.EventRTCVADStatus {
		t.Fatalf("expected only VAD status before threshold, got %#v", events)
	}
	if got := store.Current(); got != 0 {
		t.Fatalf("generation before sustained speech = %d, want 0", got)
	}

	s.handleAudioFrame(audioFrame("peer-1", 300, 1, now.Add(time.Millisecond)))
	events = drainEvents(s.downstream)
	if len(events) != 3 {
		t.Fatalf("expected speech-start, audio-start and frame events at threshold, got %#v", events)
	}
	if events[0].Kind != types.EventSpeechStart {
		t.Fatalf("expected speech start after sustained speech, got %s", events[0].Kind)
	}
	if got := store.Current(); got != 1 {
		t.Fatalf("generation after sustained speech = %d, want 1", got)
	}
}

func TestHandleAudioFrameEndsSpeechAfterSilence(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	s := &stage{
		ctx:        context.Background(),
		downstream: make(chan types.Event, 8),
		peers: map[string]*peerState{
			"peer-1": {speechThreshold: adaptiveVADMinThreshold, speechThresholdUpdatedAt: now},
		},
	}
	s.handleAudioFrame(audioFrame("peer-1", 300, 100, now))
	s.handleAudioFrame(audioFrame("peer-1", 300, 100, now.Add(100*time.Millisecond)))
	_ = drainEvents(s.downstream)

	s.handleAudioFrame(audioFrame("peer-1", 0, vadEndThreshold, now.Add(time.Second)))

	events := drainEvents(s.downstream)
	if len(events) != 4 {
		t.Fatalf("expected status, frame, speech-end and audio-end events, got %d: %#v", len(events), events)
	}
	if events[0].Kind != types.EventRTCVADStatus {
		t.Fatalf("expected status event, got %s", events[0].Kind)
	}
	if events[1].Kind != types.EventRTCSpeechAudio {
		t.Fatalf("expected silence frame to be forwarded before end, got %s", events[1].Kind)
	}
	if events[2].Kind != types.EventSpeechEnd {
		t.Fatalf("expected speech end, got %s", events[2].Kind)
	}
	audioEnd, ok := events[3].Payload.(types.RTCSpeechAudio)
	if events[3].Kind != types.EventRTCSpeechAudio || !ok || audioEnd.Type != types.RTCSpeechAudioEnd {
		t.Fatalf("expected speech audio end, got %#v", events[3])
	}
	if s.activeSpeakerID != "" {
		t.Fatalf("expected active speaker to be cleared, got %q", s.activeSpeakerID)
	}
}

func TestHandleAudioFrameBlocksOtherPeerWhileSpeakerActive(t *testing.T) {
	s := &stage{
		ctx:             context.Background(),
		downstream:      make(chan types.Event, 8),
		peers:           map[string]*peerState{},
		activeSpeakerID: "peer-1",
	}
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	s.handleAudioFrame(audioFrame("peer-2", 300, 100, now))

	if events := drainEvents(s.downstream); len(events) != 0 {
		t.Fatalf("expected no events for non-active peer, got %#v", events)
	}
	if _, ok := s.peers["peer-2"]; ok {
		t.Fatal("expected non-active peer state not to be created")
	}
}

func audioFrame(peerID string, energy int16, durationMs int, capturedAt time.Time) types.RTCPeerAudioFrame {
	return types.RTCPeerAudioFrame{
		PeerID:     peerID,
		Samples:    []int16{energy, energy},
		PCM:        []byte{byte(energy), 0, byte(energy), 0},
		SampleRate: webrtcSampleRate,
		DurationMs: durationMs,
		CapturedAt: capturedAt,
	}
}

func drainEvents(ch <-chan types.Event) []types.Event {
	var events []types.Event
	for {
		select {
		case evt := <-ch:
			events = append(events, evt)
		default:
			return events
		}
	}
}
