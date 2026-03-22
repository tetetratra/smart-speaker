package rtc

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	speech "cloud.google.com/go/speech/apiv2"
	speechpb "cloud.google.com/go/speech/apiv2/speechpb"

	"smart-speaker/internal/graph"
	types "smart-speaker/internal/types"
)

const (
	webrtcSampleRate = 48000
	webrtcChannels   = 1
	opusFrameMs      = 20

	prebufferSeconds      = 3
	vadStartThreshold     = 200
	vadEndThreshold       = 500
	sttStopDelay          = 1500 * time.Millisecond
	energySpeechThresh    = 120
	speechAudioChunkBytes = 25600
	speechModel           = "chirp_3"
	speechRegion          = "asia-northeast1"
)

type Config struct {
	IceHostIPs       []string
	SpeechProjectID  string
	SpeechRecognizer string
	SpeechLanguage   string
	SpeechCredsJSON  string
}

func NewStage(cfg Config) (*graph.Stage, error) {
	if strings.TrimSpace(cfg.SpeechRecognizer) == "" {
		cfg.SpeechRecognizer = "_"
	}
	if strings.TrimSpace(cfg.SpeechLanguage) == "" {
		cfg.SpeechLanguage = "ja-JP"
	}
	r := &stage{
		cfg:        cfg,
		upstream:   make(chan types.Event, graph.DefaultChannelBufferSize),
		downstream: make(chan types.Event, graph.DefaultChannelBufferSize),
	}
	return &graph.Stage{
		Upstream:   r.upstream,
		Downstream: r.downstream,
		Run:        r.run,
		CloseFn:    r.close,
	}, nil
}

type stage struct {
	cfg Config

	upstream   chan types.Event
	downstream chan types.Event

	ctx    context.Context
	cancel context.CancelFunc

	mu              sync.Mutex
	peers           map[string]*peerState
	activeSpeakerID string
	closed          bool

	speechClient *speech.Client
	speechStream speechpb.Speech_StreamingRecognizeClient
	speechCancel context.CancelFunc
	speechTimer  *time.Timer
}

func (s *stage) run(parent context.Context) {
	s.ctx, s.cancel = context.WithCancel(parent)
	go s.consume()
	go s.sendLoop()
}

func (s *stage) consume() {
	defer close(s.downstream)
	for {
		select {
		case <-s.ctx.Done():
			return
		case evt, ok := <-s.upstream:
			if !ok {
				return
			}
			switch evt.Kind {
			case types.EventRTCSignal:
				sig, ok := evt.Payload.(types.RTCSignal)
				if !ok {
					continue
				}
				s.handleSignal(sig)
			case types.EventRealtimeAudio:
				audio, ok := evt.Payload.(types.OutputAudio)
				if !ok {
					continue
				}
				s.handleTTSAudio(audio)
			case types.EventTTSCancel:
				s.handleTTSCancel()
			}
		}
	}
}

func (s *stage) emit(evt types.Event) {
	select {
	case <-s.ctx.Done():
		return
	case s.downstream <- evt:
	}
}

func (s *stage) close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.cancel != nil {
		s.cancel()
	}
	s.stopSpeechLocked()
	if s.speechClient != nil {
		if err := s.speechClient.Close(); err != nil {
			log.Printf("rtc: speech client close error: %v", err)
		}
		s.speechClient = nil
	}
	s.resetAllPeersLocked()
	close(s.upstream)
	return nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
