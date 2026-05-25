package stt

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	speech "cloud.google.com/go/speech/apiv2"
	speechpb "cloud.google.com/go/speech/apiv2/speechpb"

	"github.com/tetetratra/smart-speaker/internal/graph"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

const (
	sttStopDelay          = 0 * time.Millisecond
	speechAudioChunkBytes = 25600
	speechModel           = "chirp_3"
	speechRegion          = "asia-northeast1"
)

type Config struct {
	SpeechProjectID  string
	SpeechRecognizer string
	SpeechLanguage   string
	SpeechCredsJSON  string
	SpeechPhrases    []string
}

func NewStage(cfg Config) (*graph.Stage, error) {
	if strings.TrimSpace(cfg.SpeechRecognizer) == "" {
		cfg.SpeechRecognizer = "_"
	}
	if strings.TrimSpace(cfg.SpeechLanguage) == "" {
		cfg.SpeechLanguage = "ja-JP"
	}
	s := &stage{
		cfg:        cfg,
		upstream:   make(chan types.Event, graph.DefaultChannelBufferSize),
		downstream: make(chan types.Event, graph.DefaultChannelBufferSize),
	}
	return &graph.Stage{
		Upstream:   s.upstream,
		Downstream: s.downstream,
		Run:        s.run,
		CloseFn:    s.close,
	}, nil
}

type stage struct {
	cfg Config

	upstream   chan types.Event
	downstream chan types.Event

	ctx    context.Context
	cancel context.CancelFunc

	mu     sync.Mutex
	closed bool

	speechClient *speech.Client
	speechStream speechpb.Speech_StreamingRecognizeClient
	speechCancel context.CancelFunc
	speechTimer  *time.Timer
}

func (s *stage) run(parent context.Context) {
	s.ctx, s.cancel = context.WithCancel(parent)
	go s.consume()
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
			if evt.Kind != types.EventRTCSpeechAudio {
				continue
			}
			audio, ok := evt.Payload.(types.RTCSpeechAudio)
			if !ok {
				continue
			}
			s.handleSpeechAudio(audio)
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
			log.Printf("stt: speech client close error: %v", err)
		}
		s.speechClient = nil
	}
	close(s.upstream)
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
