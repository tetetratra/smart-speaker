package rtcout

import (
	"context"
	"sync"

	opus "gopkg.in/hraban/opus.v2"

	"github.com/tetetratra/smart-speaker/internal/graph"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

const (
	webrtcSampleRate = 48000
	opusFrameMs      = 20
)

type Config struct{}

func NewStage(cfg Config) (*graph.Stage, error) {
	s := &stage{
		upstream:   make(chan types.Event, graph.DefaultChannelBufferSize),
		downstream: make(chan types.Event, graph.DefaultChannelBufferSize),
		sinks:      map[string]*peerSink{},
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

	mu     sync.Mutex
	sinks  map[string]*peerSink
	closed bool
}

type peerSink struct {
	id string
	mu sync.Mutex

	writer       types.RTCPeerOutputWriter
	encoder      *opus.Encoder
	audioBuf     []int16
	connected    bool
	opusChannels int
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
			case types.EventRealtimeAudio:
				audio, ok := evt.Payload.(types.OutputAudio)
				if ok {
					s.handleTTSAudio(audio)
				}
			case types.EventRTCPeerOutputSink:
				sink, ok := evt.Payload.(types.RTCPeerOutputSink)
				if ok {
					s.handlePeerOutputSink(sink)
				}
			}
		}
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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
