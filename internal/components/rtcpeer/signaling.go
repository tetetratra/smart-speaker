package rtcpeer

import (
	"log"
	"strings"
	"sync"

	"github.com/pion/interceptor"
	"github.com/pion/webrtc/v4"

	types "github.com/tetetratra/smart-speaker/internal/types"
)

type peerState struct {
	id string
	mu sync.Mutex

	peer         *webrtc.PeerConnection
	track        *webrtc.TrackLocalStaticSample
	pendingICE   []webrtc.ICECandidateInit
	connected    bool
	opusChannels int
}

func (s *stage) handleSignal(sig types.RTCSignal) {
	switch sig.Type {
	case "webrtc.offer":
		s.handleOffer(sig)
	case "webrtc.answer":
		s.handleAnswer(sig)
	case "webrtc.ice":
		s.handleICE(sig)
	}
}

func (s *stage) handleOffer(sig types.RTCSignal) {
	clientID := normalizeClientID(sig.ClientID)
	peerState := s.getOrCreatePeer(clientID)
	s.resetPeer(peerState)

	peer, err := newPeerConnection(s.cfg.IceHostIPs)
	if err != nil {
		log.Printf("rtc: peer create error: %v", err)
		return
	}

	opusChannels := parseOpusChannels(sig.SDP)
	track, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: webrtcSampleRate, Channels: uint16(opusChannels)},
		"audio",
		"smart-speaker",
	)
	if err != nil {
		log.Printf("rtc: track create error: %v", err)
		return
	}
	if _, err := peer.AddTrack(track); err != nil {
		log.Printf("rtc: add track error: %v", err)
		return
	}

	peer.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		init := c.ToJSON()
		s.emit(types.Event{Kind: types.EventRTCSignal, Payload: types.RTCSignal{
			Type: "webrtc.ice",
			Candidate: &types.RTCIceCandidate{
				Candidate:     init.Candidate,
				SDPMid:        init.SDPMid,
				SDPMLineIndex: init.SDPMLineIndex,
			},
			ClientID: clientID,
		}})
	})
	peer.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("rtcpeer: connection state=%s", state.String())
		if state == webrtc.PeerConnectionStateClosed || state == webrtc.PeerConnectionStateFailed {
			peerState.mu.Lock()
			peerState.connected = false
			peerState.mu.Unlock()
			s.emitPeerOutputSink(clientID, nil, opusChannels, false)
		}
	})
	peer.OnTrack(func(trackRemote *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		log.Printf("rtcpeer: incoming track kind=%s codec=%s/%d channels=%d", trackRemote.Kind().String(), trackRemote.Codec().MimeType, trackRemote.Codec().ClockRate, trackRemote.Codec().Channels)
		go s.handleIncomingTrack(clientID, trackRemote)
	})

	log.Printf("rtcpeer: using opus channels=%d", opusChannels)

	peerState.mu.Lock()
	peerState.peer = peer
	peerState.track = track
	peerState.connected = true
	peerState.opusChannels = opusChannels
	peerState.mu.Unlock()
	s.emitPeerOutputSink(clientID, track, opusChannels, true)

	if err := peer.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  sig.SDP,
	}); err != nil {
		log.Printf("rtcpeer: set remote desc error: %v", err)
		return
	}
	peerState.mu.Lock()
	pendingICE := append([]webrtc.ICECandidateInit(nil), peerState.pendingICE...)
	peerState.pendingICE = nil
	peerState.mu.Unlock()
	for _, cand := range pendingICE {
		if err := peer.AddICECandidate(cand); err != nil {
			log.Printf("rtcpeer: add pending ice error: %v", err)
		}
	}

	answer, err := peer.CreateAnswer(nil)
	if err != nil {
		log.Printf("rtcpeer: create answer error: %v", err)
		return
	}
	if err := peer.SetLocalDescription(answer); err != nil {
		log.Printf("rtcpeer: set local desc error: %v", err)
		return
	}
	s.emit(types.Event{Kind: types.EventRTCSignal, Payload: types.RTCSignal{
		Type:     "webrtc.answer",
		SDP:      answer.SDP,
		ClientID: clientID,
	}})
}

func newPeerConnection(iceHostIPs []string) (*webrtc.PeerConnection, error) {
	var m webrtc.MediaEngine
	if err := m.RegisterDefaultCodecs(); err != nil {
		return nil, err
	}
	var i interceptor.Registry
	if err := webrtc.RegisterDefaultInterceptors(&m, &i); err != nil {
		return nil, err
	}
	var s webrtc.SettingEngine
	icePortMin := uint16(50000)
	icePortMax := uint16(50100)
	if err := s.SetEphemeralUDPPortRange(icePortMin, icePortMax); err != nil {
		return nil, err
	}
	log.Printf("rtcpeer: use ICE UDP port range %d-%d", icePortMin, icePortMax)
	if len(iceHostIPs) > 0 {
		log.Printf("rtcpeer: use ICE host IPs: %s", strings.Join(iceHostIPs, ","))
		s.SetNAT1To1IPs(iceHostIPs, webrtc.ICECandidateTypeHost)
	}
	api := webrtc.NewAPI(
		webrtc.WithMediaEngine(&m),
		webrtc.WithInterceptorRegistry(&i),
		webrtc.WithSettingEngine(s),
	)
	return api.NewPeerConnection(webrtc.Configuration{})
}

func (s *stage) handleAnswer(sig types.RTCSignal) {
	clientID := normalizeClientID(sig.ClientID)
	peerState := s.getPeer(clientID)
	if peerState == nil {
		return
	}
	peerState.mu.Lock()
	peer := peerState.peer
	peerState.mu.Unlock()
	if peer == nil {
		return
	}
	if err := peer.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  sig.SDP,
	}); err != nil {
		log.Printf("rtcpeer: set remote answer error: %v", err)
	}
}

func (s *stage) handleICE(sig types.RTCSignal) {
	if sig.Candidate == nil {
		return
	}
	clientID := normalizeClientID(sig.ClientID)
	init := webrtc.ICECandidateInit{
		Candidate:     sig.Candidate.Candidate,
		SDPMid:        sig.Candidate.SDPMid,
		SDPMLineIndex: sig.Candidate.SDPMLineIndex,
	}
	peerState := s.getOrCreatePeer(clientID)
	peerState.mu.Lock()
	defer peerState.mu.Unlock()
	if peerState.peer == nil {
		peerState.pendingICE = append(peerState.pendingICE, init)
		return
	}
	if err := peerState.peer.AddICECandidate(init); err != nil {
		log.Printf("rtcpeer: add ice error: %v", err)
	}
}

func parseOpusChannels(sdp string) int {
	lines := strings.Split(sdp, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "a=rtpmap:") && strings.Contains(line, "opus/48000/") {
			parts := strings.Split(line, "opus/48000/")
			if len(parts) < 2 {
				break
			}
			ch := strings.TrimSpace(parts[1])
			if ch == "1" {
				return 1
			}
			if ch == "2" {
				return 2
			}
		}
	}
	return webrtcChannels
}

func normalizeClientID(clientID string) string {
	id := strings.TrimSpace(clientID)
	if id == "" {
		return "default"
	}
	return id
}

func (s *stage) getPeer(id string) *peerState {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.peers == nil {
		return nil
	}
	return s.peers[id]
}

func (s *stage) getOrCreatePeer(id string) *peerState {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.peers == nil {
		s.peers = map[string]*peerState{}
	}
	if peer, ok := s.peers[id]; ok && peer != nil {
		return peer
	}
	peer := &peerState{id: id}
	s.peers[id] = peer
	return peer
}

func (s *stage) resetPeer(peer *peerState) {
	if peer == nil {
		return
	}
	s.emitPeerOutputSink(peer.id, nil, peer.opusChannels, false)
	peer.mu.Lock()
	if peer.peer != nil {
		_ = peer.peer.Close()
		peer.peer = nil
	}
	peer.track = nil
	peer.pendingICE = nil
	peer.connected = false
	peer.mu.Unlock()
}

func (s *stage) resetAllPeersLocked() {
	for _, peer := range s.peers {
		s.emitPeerOutputSink(peer.id, nil, peer.opusChannels, false)
		peer.mu.Lock()
		if peer.peer != nil {
			_ = peer.peer.Close()
			peer.peer = nil
		}
		peer.track = nil
		peer.pendingICE = nil
		peer.connected = false
		peer.mu.Unlock()
	}
	s.peers = nil
}

func (s *stage) emitPeerOutputSink(peerID string, writer types.RTCPeerOutputWriter, opusChannels int, connected bool) {
	s.emit(types.Event{
		Kind: types.EventRTCPeerOutputSink,
		Payload: types.RTCPeerOutputSink{
			PeerID:       peerID,
			Writer:       writer,
			OpusChannels: opusChannels,
			Connected:    connected,
		},
	})
}
