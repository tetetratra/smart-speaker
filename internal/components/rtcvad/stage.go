package rtcvad

import (
	"context"
	"sync"
	"time"

	"github.com/tetetratra/smart-speaker/internal/graph"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

const (
	webrtcSampleRate = 48000

	prebufferSeconds  = 3
	vadStartThreshold = 200
	vadEndThreshold   = 500

	adaptiveVADHistoryWindow            = time.Minute
	adaptiveVADThresholdRefreshInterval = time.Second
	adaptiveVADStatusEmitInterval       = 250 * time.Millisecond
	adaptiveVADThresholdOffset          = 50
	adaptiveVADMinThreshold             = 50
)

type Config struct{}

func NewStage(cfg Config) (*graph.Stage, error) {
	s := &stage{
		upstream:   make(chan types.Event, graph.DefaultChannelBufferSize),
		downstream: make(chan types.Event, graph.DefaultChannelBufferSize),
		peers:      map[string]*peerState{},
	}
	return &graph.Stage{
		Upstream:   s.upstream,
		Downstream: s.downstream,
		Run:        s.run,
		CloseFn:    s.close,
	}, nil
}

type stage struct {
	upstream   chan types.Event
	downstream chan types.Event

	ctx    context.Context
	cancel context.CancelFunc

	mu              sync.Mutex
	peers           map[string]*peerState
	activeSpeakerID string
	closed          bool
}

type peerState struct {
	mu sync.Mutex

	inputSampleRate int
	prebuffer       *pcmRingBuffer
	speechActive    bool
	voicedMs        int
	silenceMs       int

	backgroundEnergies       []energySample
	speechThreshold          int
	speechThresholdUpdatedAt time.Time
	lastVADStatusSentAt      time.Time
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
			if evt.Kind != types.EventRTCPeerAudioFrame {
				continue
			}
			frame, ok := evt.Payload.(types.RTCPeerAudioFrame)
			if !ok {
				continue
			}
			s.handleAudioFrame(frame)
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
	close(s.upstream)
	return nil
}
