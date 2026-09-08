package rtcpeer

import (
	"context"
	"fmt"
	"net/netip"
	"sync"

	"github.com/tetetratra/smart-speaker/internal/graph"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

const (
	webrtcSampleRate = 48000
	webrtcChannels   = 1
)

type Config struct {
	IceAdvertiseIPs []string
}

func NewStage(cfg Config) (*graph.Stage, error) {
	if err := validateICEAdvertiseIPs(cfg.IceAdvertiseIPs); err != nil {
		return nil, err
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

func validateICEAdvertiseIPs(values []string) error {
	tailscaleIPv4 := netip.MustParsePrefix("100.64.0.0/10")
	for _, value := range values {
		addr, err := netip.ParseAddr(value)
		if err != nil {
			return fmt.Errorf("rtcpeer: invalid ICE advertise IP %q: %w", value, err)
		}
		addr = addr.Unmap()
		if !addr.Is4() {
			return fmt.Errorf("rtcpeer: ICE advertise IP must be IPv4: %q", value)
		}
		if !addr.IsPrivate() && !tailscaleIPv4.Contains(addr) {
			return fmt.Errorf("rtcpeer: ICE advertise IP must be private or Tailscale IPv4: %q", value)
		}
	}
	return nil
}

type stage struct {
	cfg Config

	upstream   chan types.Event
	downstream chan types.Event

	ctx    context.Context
	cancel context.CancelFunc

	mu     sync.Mutex
	peers  map[string]*peerState
	closed bool
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
			if evt.Kind != types.EventRTCSignal {
				continue
			}
			sig, ok := evt.Payload.(types.RTCSignal)
			if !ok {
				continue
			}
			s.handleSignal(sig)
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
